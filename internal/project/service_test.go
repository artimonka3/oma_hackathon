package project

import (
	"context"
	"errors"
	"testing"

	"oma/internal/apperr"
	"oma/internal/events"
)

func newTestService(t *testing.T) (*Service, UserID) {
	t.Helper()
	s := NewService(NewMemoryRepo(), events.NewLog())
	return s, "client_demo"
}

func TestCreateProjectRequiresGoal(t *testing.T) {
	s, owner := newTestService(t)
	if _, err := s.Create(context.Background(), owner, CreateInput{}); err == nil {
		t.Fatal("ожидалась ошибка для пустой цели")
	}
	if _, err := s.Create(context.Background(), owner, CreateInput{Goal: "Гостиная", ExecutionMode: "wrong"}); err == nil {
		t.Fatal("ожидалась ошибка для неизвестного режима исполнения")
	}
	p, err := s.Create(context.Background(), owner, CreateInput{Goal: "Обновить гостиную", ExecutionMode: "self"})
	if err != nil {
		t.Fatalf("не ожидалась ошибка: %v", err)
	}
	if p.Version != 1 {
		t.Fatalf("версия нового проекта = %d, ожидалась 1", p.Version)
	}
}

func TestUpdateVersionConflict(t *testing.T) {
	s, owner := newTestService(t)
	p, _ := s.Create(context.Background(), owner, CreateInput{Goal: "Гостиная"})
	wrong := UpdateInput{ExpectedVersion: 99}
	_, err := s.Update(context.Background(), owner, p.ID, wrong)
	var ae *apperr.Error
	if !errors.As(err, &ae) || ae.Code != "VERSION_CONFLICT" {
		t.Fatalf("ожидался VERSION_CONFLICT, получено: %v", err)
	}
	newGoal := "Гостиная и прихожая"
	upd, err := s.Update(context.Background(), owner, p.ID, UpdateInput{Goal: &newGoal, ExpectedVersion: p.Version})
	if err != nil {
		t.Fatalf("не ожидалась ошибка: %v", err)
	}
	if upd.Version != 2 || upd.Goal != newGoal {
		t.Fatalf("обновление применилось некорректно: %+v", upd)
	}
}

func TestForeignActorForbidden(t *testing.T) {
	s, owner := newTestService(t)
	p, _ := s.Create(context.Background(), owner, CreateInput{Goal: "Гостиная"})
	_, err := s.GetForActor(context.Background(), "intruder", p.ID)
	var ae *apperr.Error
	if !errors.As(err, &ae) || ae.Code != "FORBIDDEN" {
		t.Fatalf("ожидался FORBIDDEN, получено: %v", err)
	}
	_, err = s.AddRoom(context.Background(), "foreman_demo_01", p.ID, AddRoomInput{Name: "Спальня", WidthMM: 3000, LengthMM: 4000, HeightMM: 2700})
	if !errors.As(err, &ae) || ae.Code != "FORBIDDEN" {
		t.Fatalf("ожидался FORBIDDEN для добавления помещения не владельцем: %v", err)
	}
}

func TestSuggestedLotDoesNotDuplicate(t *testing.T) {
	s, owner := newTestService(t)
	p, _ := s.Create(context.Background(), owner, CreateInput{Goal: "Гостиная"})
	items := []HistorySuggestion{{SKU: "PAINT-MATT-WHITE", Title: "Краска", Amount: "4 л"}}
	first, err := s.SuggestInventory(context.Background(), owner, p.ID, items)
	if err != nil || len(first) != 1 {
		t.Fatalf("первая подсказка: %v %v", first, err)
	}
	second, err := s.SuggestInventory(context.Background(), owner, p.ID, items)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 0 {
		t.Fatalf("повторная подсказка не должна дублировать партии: %v", second)
	}
	lot, err := s.ConfirmInventory(context.Background(), owner, p.ID, first[0].ID, "3 л")
	if err != nil {
		t.Fatal(err)
	}
	if lot.Status != LotConfirmed {
		t.Fatalf("партия не подтверждена: %s", lot.Status)
	}
}
