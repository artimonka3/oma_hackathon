package prorab

import (
	"context"
	"errors"
	"testing"

	"oma/internal/apperr"
	"oma/internal/events"
	"oma/internal/project"
)

type fixture struct {
	svc     *Service
	client  project.UserID
	foreman project.UserID
	pid     string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	projects := project.NewService(project.NewMemoryRepo(), events.NewLog())
	svc := NewService(projects, events.NewLog(), []Foreman{{
		ID: "foreman_demo_01", Name: "Демо-прораб", Specialities: []string{"painting"},
		Region: "Minsk", Verified: true, Demo: true,
	}})
	client := project.UserID("client_demo")
	foreman := project.UserID("foreman_demo_01")
	p, err := projects.Create(context.Background(), client, project.CreateInput{
		Goal: "Обновить гостиную", ExecutionMode: project.ModeForeman,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projects.AddRoom(context.Background(), client, p.ID, project.AddRoomInput{
		Name: "Гостиная", WidthMM: 4000, LengthMM: 5000, HeightMM: 2700, DimensionsConfirmed: true,
	}); err != nil {
		t.Fatal(err)
	}
	return &fixture{svc: svc, client: client, foreman: foreman, pid: string(p.ID)}
}

func (f *fixture) confirmedPlan(t *testing.T) (planID, stageID string) {
	t.Helper()
	plan, err := f.svc.CreateWorkPlan(context.Background(), f.client, f.pid, CreatePlanInput{
		Stages: []StageInput{{Title: "Подготовка стен", Required: true, Weight: 60}, {Title: "Покраска", Required: true, Weight: 40}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.ConfirmWorkPlan(context.Background(), f.client, plan.ID); err != nil {
		t.Fatal(err)
	}
	return plan.ID, plan.StageIDs[0]
}

func TestPreflightAndReviewFlow(t *testing.T) {
	f := newFixture(t)
	report, err := f.svc.Preflight(context.Background(), f.client, f.pid)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "ready" {
		t.Fatalf("проект должен быть готов к проверке: %+v", report)
	}
	rev1, created, err := f.svc.CreateReview(context.Background(), f.client, f.pid, CreateReviewInput{ForemanID: DefaultForemanID})
	if err != nil || !created {
		t.Fatalf("проверка не создана: %v", err)
	}
	// Повторный запрос того же состава и исполнителя не создаёт дубли.
	rev2, created, err := f.svc.CreateReview(context.Background(), f.client, f.pid, CreateReviewInput{ForemanID: DefaultForemanID})
	if err != nil {
		t.Fatal(err)
	}
	if created || rev2.ID != rev1.ID {
		t.Fatal("повторный запрос проверки не должен создавать новую карточку")
	}
	// Решение может принять только назначенный исполнитель.
	_, err = f.svc.ReviewDecision(context.Background(), f.client, rev1.ID, ReviewDecisionInput{Action: "approve_current"})
	var ae *apperr.Error
	if !errors.As(err, &ae) || ae.Code != "FORBIDDEN" {
		t.Fatalf("ожидался FORBIDDEN для решения клиента: %v", err)
	}
	approved, err := f.svc.ReviewDecision(context.Background(), f.foreman, rev1.ID, ReviewDecisionInput{Action: "approve_current"})
	if err != nil {
		t.Fatal(err)
	}
	if approved.Status != ReviewApproved {
		t.Fatalf("проверка не подтверждена: %s", approved.Status)
	}
	// Повторное решение уже закрытой проверки отклоняется.
	_, err = f.svc.ReviewDecision(context.Background(), f.foreman, rev1.ID, ReviewDecisionInput{Action: "approve_current"})
	if !errors.As(err, &ae) || ae.Code != "INVALID_TRANSITION" {
		t.Fatalf("ожидался INVALID_TRANSITION, получено: %v", err)
	}
}

func TestStageAcceptanceRules(t *testing.T) {
	f := newFixture(t)
	_, stageID := f.confirmedPlan(t)

	st, err := f.svc.StartStage(context.Background(), f.foreman, stageID)
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != StageInProgress {
		t.Fatalf("этап не запущен: %s", st.Status)
	}
	// Клиент не может начать этап.
	if _, err := f.svc.StartStage(context.Background(), f.client, stageID); err == nil {
		t.Fatal("клиент не должен запускать этап")
	}
	report, _, err := f.svc.SubmitReport(context.Background(), f.foreman, stageID, ReportInput{Items: []string{"Стены подготовлены"}})
	if err != nil {
		t.Fatal(err)
	}
	// Прораб не может принять свой этап.
	_, err = f.svc.StageDecision(context.Background(), f.foreman, stageID, StageDecisionInput{
		ExpectedVersion: 2, ReportID: report.ID, Decision: "accept", AcceptanceConfirmed: true,
	})
	var ae *apperr.Error
	if !errors.As(err, &ae) || ae.Code != "FORBIDDEN" {
		t.Fatalf("ожидался FORBIDDEN для приёмки прорабом: %v", err)
	}
	// Приёмка без явного подтверждения не проходит.
	_, err = f.svc.StageDecision(context.Background(), f.client, stageID, StageDecisionInput{
		ExpectedVersion: 2, ReportID: report.ID, Decision: "accept", AcceptanceConfirmed: false,
	})
	if !errors.As(err, &ae) || ae.Code != "INVALID_INPUT" {
		t.Fatalf("ожидался INVALID_INPUT без acceptance_confirmed: %v", err)
	}
	// Замечание переводит этап в changes_requested, отчёт пересдаётся.
	if _, err := f.svc.StageDecision(context.Background(), f.client, stageID, StageDecisionInput{
		ExpectedVersion: 2, ReportID: report.ID, Decision: "request_changes", Comment: "Закруглить углы",
	}); err != nil {
		t.Fatal(err)
	}
	report2, _, err := f.svc.SubmitReport(context.Background(), f.foreman, stageID, ReportInput{Items: []string{"Замечание устранено"}})
	if err != nil {
		t.Fatal(err)
	}
	if report2.Version != 2 {
		t.Fatalf("новый отчёт должен быть второй версией: %d", report2.Version)
	}
	// Решение по устаревшей версии отчёта отклоняется.
	_, err = f.svc.StageDecision(context.Background(), f.client, stageID, StageDecisionInput{
		ExpectedVersion: 4, ReportID: report.ID, Decision: "accept", AcceptanceConfirmed: true,
	})
	if !errors.As(err, &ae) || ae.Code != "REPORT_CHANGED" {
		t.Fatalf("ожидался REPORT_CHANGED: %v", err)
	}
	accepted, err := f.svc.StageDecision(context.Background(), f.client, stageID, StageDecisionInput{
		ExpectedVersion: 4, ReportID: report2.ID, Decision: "accept", AcceptanceConfirmed: true,
		Comment: "Замечания устранены, этап принимаю",
	})
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Status != StageAccepted {
		t.Fatalf("этап не принят: %s", accepted.Status)
	}
	if got := f.svc.AcceptanceProgress(f.pid); got != 60 {
		t.Fatalf("прогресс приёмки = %d, ожидался 60 (вес принятого этапа из 100)", got)
	}
}
