package collaboration

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

// maxTeamsPerPage 限制单次列出的团队数，避免组织规模增长后端点退化为慢查询。
const maxTeamsPerPage = 500

func (s *Service) ListTeams(ctx context.Context, organizationID, userID uint64) ([]TeamView, error) {
	// 组织归属只解析一次：此前循环内每个团队都会重复调用（内部是两次查询）。
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	var teams []models.Team
	if err := s.db.WithContext(ctx).
		Where("organization_id = ?", organizationID).
		Order("name ASC").
		Limit(maxTeamsPerPage).
		Find(&teams).Error; err != nil {
		return nil, err
	}
	if len(teams) == 0 {
		return []TeamView{}, nil
	}

	teamIDs := make([]uint64, 0, len(teams))
	for _, t := range teams {
		teamIDs = append(teamIDs, t.ID)
	}

	// 一次聚合取代 N 次 COUNT。
	counts := make(map[uint64]int64, len(teamIDs))
	var rows []struct {
		TeamID uint64 `gorm:"column:team_id"`
		Total  int64  `gorm:"column:total"`
	}
	if err := s.db.WithContext(ctx).
		Model(&models.TeamMember{}).
		Select("team_id, COUNT(*) AS total").
		Where("team_id IN ?", teamIDs).
		Group("team_id").
		Scan(&rows).Error; err != nil {
		s.logger.Warn().Err(err).Msg("failed to count team members in batch")
	} else {
		for _, r := range rows {
			counts[r.TeamID] = r.Total
		}
	}

	// 一次批量查询取代 N 次成员列表查询。
	membersByTeam, err := s.listTeamMembersBatch(ctx, organizationID, teamIDs)
	if err != nil {
		s.logger.Warn().Err(err).Msg("failed to load team members in batch")
	}

	result := make([]TeamView, 0, len(teams))
	for _, team := range teams {
		result = append(result, TeamView{
			Team:        team,
			MemberCount: counts[team.ID],
			Members:     membersByTeam[team.ID],
		})
	}
	return result, nil
}

// listTeamMembersBatch 一次性取出多个团队的成员（含用户邮箱与昵称）。
func (s *Service) listTeamMembersBatch(ctx context.Context, organizationID uint64, teamIDs []uint64) (map[uint64][]TeamMemberView, error) {
	out := make(map[uint64][]TeamMemberView, len(teamIDs))
	if len(teamIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		TeamMemberView
		TeamID uint64 `gorm:"column:team_id"`
	}
	if err := s.db.WithContext(ctx).
		Table("team_members").
		Select("team_members.team_id AS team_id, team_members.*, users.email AS email, users.display_name AS display_name").
		Joins("JOIN users ON users.id = team_members.user_id").
		Joins("JOIN teams ON teams.id = team_members.team_id").
		Where("teams.organization_id = ? AND team_members.team_id IN ?", organizationID, teamIDs).
		Order("users.display_name ASC, users.email ASC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.TeamID] = append(out[r.TeamID], r.TeamMemberView)
	}
	return out, nil
}

func (s *Service) CreateTeam(ctx context.Context, organizationID, actorID uint64, input TeamInput) (*TeamView, error) {
	if _, err := s.requireOrganizationAdmin(ctx, organizationID, actorID); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, errors.New("team name required")
	}
	team := models.Team{
		OrganizationID: organizationID,
		Name:           name,
		Slug:           uniqueSlug(name, organizationID),
		Description:    strings.TrimSpace(input.Description),
		CreatedBy:      actorID,
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&team).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.TeamMember{TeamID: team.ID, UserID: actorID, Role: models.OrganizationRoleOwner, JoinedAt: time.Now()}).Error; err != nil {
			return err
		}
		return s.recordOrganizationAuditTx(ctx, tx, organizationID, actorID, "organization.team.created", "team", strconv.FormatUint(team.ID, 10), map[string]any{"name": team.Name})
	})
	if err != nil {
		return nil, err
	}
	s.invalidateOrganizationAdminSummary(ctx, organizationID)
	return s.getTeamView(ctx, organizationID, actorID, team.ID)
}

func (s *Service) UpdateTeam(ctx context.Context, organizationID, actorID, teamID uint64, input TeamInput) (*TeamView, error) {
	if _, err := s.requireOrganizationAdmin(ctx, organizationID, actorID); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, errors.New("team name required")
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.Team{}).
			Where("organization_id = ? AND id = ?", organizationID, teamID).
			Updates(map[string]any{"name": name, "slug": uniqueSlug(name, organizationID), "description": strings.TrimSpace(input.Description), "updated_at": time.Now()}).Error; err != nil {
			return err
		}
		return s.recordOrganizationAuditTx(ctx, tx, organizationID, actorID, "organization.team.updated", "team", strconv.FormatUint(teamID, 10), map[string]any{"name": name})
	})
	if err != nil {
		return nil, err
	}
	s.invalidateOrganizationAdminSummary(ctx, organizationID)
	return s.getTeamView(ctx, organizationID, actorID, teamID)
}

func (s *Service) DeleteTeam(ctx context.Context, organizationID, actorID, teamID uint64) error {
	if _, err := s.requireOrganizationAdmin(ctx, organizationID, actorID); err != nil {
		return err
	}
	var team models.Team
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", organizationID, teamID).Take(&team).Error; err != nil {
		return err
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("team_id = ?", teamID).Delete(&models.TeamMember{}).Error; err != nil {
			return err
		}
		if err := tx.Where("organization_id = ? AND id = ?", organizationID, teamID).Delete(&models.Team{}).Error; err != nil {
			return err
		}
		return s.recordOrganizationAuditTx(ctx, tx, organizationID, actorID, "organization.team.deleted", "team", strconv.FormatUint(teamID, 10), map[string]any{"name": team.Name})
	})
	if err == nil {
		s.invalidateOrganizationAdminSummary(ctx, organizationID)
	}
	return err
}

func (s *Service) ListTeamMembers(ctx context.Context, organizationID, userID, teamID uint64) ([]TeamMemberView, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	var members []TeamMemberView
	err := s.db.WithContext(ctx).
		Table("team_members").
		Select("team_members.*, users.email AS email, users.display_name AS display_name").
		Joins("JOIN users ON users.id = team_members.user_id").
		Joins("JOIN teams ON teams.id = team_members.team_id").
		Where("teams.organization_id = ? AND team_members.team_id = ?", organizationID, teamID).
		Order("users.display_name ASC, users.email ASC").
		Find(&members).Error
	return members, err
}

func (s *Service) AddTeamMember(ctx context.Context, organizationID, actorID, teamID, targetUserID uint64) (*TeamView, error) {
	if _, err := s.requireOrganizationAdmin(ctx, organizationID, actorID); err != nil {
		return nil, err
	}
	var orgMember models.OrganizationMember
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND user_id = ?", organizationID, targetUserID).Take(&orgMember).Error; err != nil {
		return nil, err
	}
	if err := s.ensureTeamBelongsToOrg(ctx, organizationID, teamID); err != nil {
		return nil, err
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		member := models.TeamMember{TeamID: teamID, UserID: targetUserID, Role: orgMember.Role, JoinedAt: time.Now()}
		if err := tx.Where("team_id = ? AND user_id = ?", teamID, targetUserID).FirstOrCreate(&member).Error; err != nil {
			return err
		}
		return s.recordOrganizationAuditTx(ctx, tx, organizationID, actorID, "organization.team.member_added", "team", strconv.FormatUint(teamID, 10), map[string]any{"user_id": targetUserID})
	})
	if err != nil {
		return nil, err
	}
	s.invalidateOrganizationAdminSummary(ctx, organizationID)
	return s.getTeamView(ctx, organizationID, actorID, teamID)
}

func (s *Service) RemoveTeamMember(ctx context.Context, organizationID, actorID, teamID, targetUserID uint64) (*TeamView, error) {
	if _, err := s.requireOrganizationAdmin(ctx, organizationID, actorID); err != nil {
		return nil, err
	}
	if err := s.ensureTeamBelongsToOrg(ctx, organizationID, teamID); err != nil {
		return nil, err
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("team_id = ? AND user_id = ?", teamID, targetUserID).Delete(&models.TeamMember{}).Error; err != nil {
			return err
		}
		return s.recordOrganizationAuditTx(ctx, tx, organizationID, actorID, "organization.team.member_removed", "team", strconv.FormatUint(teamID, 10), map[string]any{"user_id": targetUserID})
	})
	if err != nil {
		return nil, err
	}
	s.invalidateOrganizationAdminSummary(ctx, organizationID)
	return s.getTeamView(ctx, organizationID, actorID, teamID)
}

func (s *Service) ensureAnotherOwner(ctx context.Context, organizationID, targetUserID uint64) error {
	var ownerCount int64
	if err := s.db.WithContext(ctx).Model(&models.OrganizationMember{}).
		Where("organization_id = ? AND role = ? AND user_id <> ?", organizationID, models.OrganizationRoleOwner, targetUserID).
		Count(&ownerCount).Error; err != nil {
		return err
	}
	if ownerCount == 0 {
		return errors.New("organization must keep at least one owner")
	}
	return nil
}

func (s *Service) getTeamView(ctx context.Context, organizationID, userID, teamID uint64) (*TeamView, error) {
	var team models.Team
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", organizationID, teamID).Take(&team).Error; err != nil {
		return nil, err
	}
	view := &TeamView{Team: team}
	if err := s.db.WithContext(ctx).Model(&models.TeamMember{}).Where("team_id = ?", teamID).Count(&view.MemberCount).Error; err != nil {
		s.logger.Warn().Err(err).Uint64("team_id", teamID).Msg("failed to count team members")
	}
	members, _ := s.ListTeamMembers(ctx, organizationID, userID, teamID)
	view.Members = members
	return view, nil
}

func (s *Service) ensureTeamBelongsToOrg(ctx context.Context, organizationID, teamID uint64) error {
	var count int64
	if err := s.db.WithContext(ctx).Model(&models.Team{}).Where("organization_id = ? AND id = ?", organizationID, teamID).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
