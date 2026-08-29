package collaboration

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/allcallall/backend/internal/models"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// countingGormLogger 统计 ListTeams 期间执行的 SQL 条数（每次 Trace 计一次）。
type countingGormLogger struct {
	count int
}

func (c *countingGormLogger) LogMode(gormlogger.LogLevel) gormlogger.Interface { return c }
func (c *countingGormLogger) Info(context.Context, string, ...interface{})     {}
func (c *countingGormLogger) Warn(context.Context, string, ...interface{})     {}
func (c *countingGormLogger) Error(context.Context, string, ...interface{})    {}
func (c *countingGormLogger) Trace(_ context.Context, _ time.Time, _ func() (string, int64), _ error) {
	c.count++
}

// TestListTeamsAvoidsNPlusOneAndReturnsCounts 验证两件事：
//  1. N+1 已消除 —— ListTeams 的 SQL 条数与团队数解耦（旧实现≈1+3N，新实现个位数）。
//  2. 正确性 —— 每个团队的成员计数与成员明细与实际写入一致。
func TestListTeamsAvoidsNPlusOneAndReturnsCounts(t *testing.T) {
	svc, db, _ := newServiceTestEnv(t)
	ctx := context.Background()

	owner := createTestUser(t, db, "nplus1-owner@example.com", "Owner")
	alice := createTestUser(t, db, "nplus1-alice@example.com", "Alice")
	bob := createTestUser(t, db, "nplus1-bob@example.com", "Bob")
	org, err := svc.CreateOrganization(ctx, owner.ID, "N+1 Org")
	if err != nil {
		t.Fatalf("create organization failed: %v", err)
	}
	addOrgMember(t, db, org.ID, alice.ID, models.OrganizationRoleMember)
	addOrgMember(t, db, org.ID, bob.ID, models.OrganizationRoleMember)

	const teamCount = 20
	for i := 0; i < teamCount; i++ {
		name := "Team-" + strconv.Itoa(i)
		tm, err := svc.CreateTeam(ctx, org.ID, owner.ID, TeamInput{Name: name})
		if err != nil {
			t.Fatalf("create team %s failed: %v", name, err)
		}
		// 偶数团队再加 alice，奇数团队仅 owner。
		if i%2 == 0 {
			if _, err := svc.AddTeamMember(ctx, org.ID, owner.ID, tm.ID, alice.ID); err != nil {
				t.Fatalf("add team member failed: %v", err)
			}
		}
	}

	// 用计数 logger 包裹 session，统计 ListTeams 的 SQL 条数。
	clog := &countingGormLogger{}
	orig := svc.db
	svc.db = orig.Session(&gorm.Session{Logger: clog})
	teams, err := svc.ListTeams(ctx, org.ID, owner.ID)
	svc.db = orig
	if err != nil {
		t.Fatalf("list teams failed: %v", err)
	}
	// CreateOrganization 会建一个默认 "General" 团队，故总数为 teamCount+1。
	if len(teams) != teamCount+1 {
		t.Fatalf("expected %d teams (incl. default General), got %d", teamCount+1, len(teams))
	}

	// 旧实现约为 1 + 3*N = 61 条；新实现应为个位数（< 10）。
	if clog.count >= teamCount {
		t.Fatalf("suspected N+1: ListTeams issued %d SQL statements for %d teams (was %d)", clog.count, teamCount, 1+3*teamCount)
	}

	byName := make(map[string]TeamView, len(teams))
	for _, tm := range teams {
		byName[tm.Team.Name] = tm
	}
	for i := 0; i < teamCount; i++ {
		name := "Team-" + strconv.Itoa(i)
		tm, ok := byName[name]
		if !ok {
			t.Fatalf("missing team %s in ListTeams result", name)
		}
		want := int64(1) // owner 是每个团队的创建者成员
		if i%2 == 0 {
			want = 2
		}
		if tm.MemberCount != want {
			t.Fatalf("%s: expected %d members, got %d", name, want, tm.MemberCount)
		}
		if int64(len(tm.Members)) != want {
			t.Fatalf("%s: members slice len %d != expected %d", name, len(tm.Members), want)
		}
		emails := make(map[string]bool, len(tm.Members))
		for _, m := range tm.Members {
			emails[m.Email] = true
		}
		if !emails["nplus1-owner@example.com"] {
			t.Fatalf("%s: owner missing from members: %v", name, memberEmailsOf(tm.Members))
		}
		if i%2 == 0 && !emails["nplus1-alice@example.com"] {
			t.Fatalf("%s: alice missing from members: %v", name, memberEmailsOf(tm.Members))
		}
		if i%2 != 0 && emails["nplus1-alice@example.com"] {
			t.Fatalf("%s: alice should not be a member", name)
		}
	}
}

func memberEmailsOf(ms []TeamMemberView) []string {
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.Email)
	}
	return out
}
