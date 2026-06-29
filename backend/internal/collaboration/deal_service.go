package collaboration

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/allcallall/backend/internal/models"
)

func (s *Service) ListPipelines(ctx context.Context, organizationID, userID uint64) ([]PipelineView, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	var pipelines []models.Pipeline
	if err := s.db.WithContext(ctx).Where("organization_id = ?", organizationID).Order("id ASC").Find(&pipelines).Error; err != nil {
		return nil, err
	}
	result := make([]PipelineView, 0, len(pipelines))
	for _, pipeline := range pipelines {
		var stages []models.PipelineStage
		_ = s.db.WithContext(ctx).Where("pipeline_id = ?", pipeline.ID).Order("position ASC").Find(&stages).Error
		result = append(result, PipelineView{Pipeline: pipeline, Stages: stages})
	}
	return result, nil
}

func (s *Service) ListDeals(ctx context.Context, organizationID, userID uint64) ([]DealView, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	type row struct {
		models.Deal
		StageName string `gorm:"column:stage_name"`
	}
	var rows []row
	err := s.db.WithContext(ctx).
		Table("deals").
		Select("deals.*, pipeline_stages.name AS stage_name").
		Joins("LEFT JOIN pipeline_stages ON pipeline_stages.id = deals.stage_id").
		Where("deals.organization_id = ?", organizationID).
		Order("deals.updated_at DESC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make([]DealView, 0, len(rows))
	for _, item := range rows {
		result = append(result, DealView{Deal: item.Deal, StageName: item.StageName})
	}
	return result, nil
}

func (s *Service) CreateDeal(ctx context.Context, organizationID, userID uint64, input DealInput) (*DealView, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	pipeline, firstStage, err := s.getDefaultPipeline(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	deal := &models.Deal{
		OrganizationID: organizationID,
		PipelineID:     pipeline.ID,
		StageID:        firstStage,
		OwnerID:        userID,
		Title:          strings.TrimSpace(input.Title),
		Description:    strings.TrimSpace(input.Description),
		Status:         models.DealStatusOpen,
		ValueCents:     input.ValueCents,
		Currency:       defaultString(strings.TrimSpace(input.Currency), "USD"),
	}
	if input.StageID != nil {
		deal.StageID = input.StageID
	}
	if deal.Title == "" {
		return nil, errors.New("deal title required")
	}
	if err := s.db.WithContext(ctx).Create(deal).Error; err != nil {
		return nil, err
	}
	_ = s.recordDealActivity(ctx, organizationID, deal.ID, userID, "deal.created", "deal", strconv.FormatUint(deal.ID, 10), fmt.Sprintf("Created deal %s", deal.Title), nil)
	return s.GetDeal(ctx, organizationID, userID, deal.ID)
}

func (s *Service) GetDeal(ctx context.Context, organizationID, userID, dealID uint64) (*DealView, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	type row struct {
		models.Deal
		StageName string `gorm:"column:stage_name"`
	}
	var item row
	err := s.db.WithContext(ctx).
		Table("deals").
		Select("deals.*, pipeline_stages.name AS stage_name").
		Joins("LEFT JOIN pipeline_stages ON pipeline_stages.id = deals.stage_id").
		Where("deals.organization_id = ? AND deals.id = ?", organizationID, dealID).
		Take(&item).Error
	if err != nil {
		return nil, err
	}
	view := &DealView{Deal: item.Deal, StageName: item.StageName}
	return view, nil
}

func (s *Service) UpdateDeal(ctx context.Context, organizationID, userID, dealID uint64, input DealUpdateInput) (*DealView, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	var deal models.Deal
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", organizationID, dealID).Take(&deal).Error; err != nil {
		return nil, err
	}
	if input.Title != nil {
		deal.Title = strings.TrimSpace(*input.Title)
	}
	if input.Description != nil {
		deal.Description = strings.TrimSpace(*input.Description)
	}
	if input.Status != nil {
		deal.Status = strings.TrimSpace(*input.Status)
	}
	if input.ValueCents != nil {
		deal.ValueCents = *input.ValueCents
	}
	if input.Currency != nil && strings.TrimSpace(*input.Currency) != "" {
		deal.Currency = strings.TrimSpace(*input.Currency)
	}
	if input.StageID != nil {
		deal.StageID = input.StageID
	}
	if err := s.db.WithContext(ctx).Save(&deal).Error; err != nil {
		return nil, err
	}
	_ = s.recordDealActivity(ctx, organizationID, deal.ID, userID, "deal.updated", "deal", strconv.FormatUint(deal.ID, 10), fmt.Sprintf("Updated deal %s", deal.Title), nil)
	return s.GetDeal(ctx, organizationID, userID, deal.ID)
}

func (s *Service) AddDealContact(ctx context.Context, organizationID, userID, dealID, contactID uint64) error {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return err
	}
	item := models.DealContact{
		DealID:    dealID,
		ContactID: contactID,
	}
	if err := s.db.WithContext(ctx).Where("deal_id = ? AND contact_id = ?", dealID, contactID).FirstOrCreate(&item).Error; err != nil {
		return err
	}
	return s.recordDealActivity(ctx, organizationID, dealID, userID, "deal.contact_added", "contact", strconv.FormatUint(contactID, 10), "Linked contact to deal", nil)
}

func (s *Service) ListDealActivities(ctx context.Context, organizationID, userID, dealID uint64) ([]models.DealActivity, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	var activities []models.DealActivity
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND deal_id = ?", organizationID, dealID).Order("created_at DESC").Find(&activities).Error; err != nil {
		return nil, err
	}
	return activities, nil
}
