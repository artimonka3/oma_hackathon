// Package ai — заменяемое подключение диалоговой и мультимодальной модели.
//
// Модель возвращает только структурированный черновик; ответ проходит
// валидацию доменными правилами до записи в проект (раздел 8 ТЗ).
// StubProvider используется для минимального запуска без внешней модели.
package ai

import "context"

// Variant — один объяснимый вариант дизайна из черновика модели.
type Variant struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Summary string   `json:"summary"`
	SKUs    []string `json:"skus"`
}

// Draft — структурированный черновик модели.
type Draft struct {
	Variants  []Variant `json:"variants"`
	Questions []string  `json:"questions"`
	Mode      string    `json:"mode"` // live | demo
}

// PrepareRequest — вход задания подготовки вариантов.
type PrepareRequest struct {
	ProjectID   string   `json:"project_id"`
	Goal        string   `json:"goal"`
	RoomNames   []string `json:"room_names"`
	AllowedSKUs []string `json:"allowed_skus"`
}

// Provider — контракт внешней модели. Заменяемый интерфейс: конкретный
// провайдер подключается без изменения доменного кода.
type Provider interface {
	PrepareVariants(ctx context.Context, req PrepareRequest) (Draft, error)
}

// StubProvider — заглушка для минимального запуска (AI_MODE=demo).
type StubProvider struct{}

// NewStub возвращает заглушку провайдера.
func NewStub() Provider { return StubProvider{} }

// PrepareVariants возвращает подготовленный демо-черновик с тремя
// объяснимыми вариантами; ответ помечен признаком demo.
func (StubProvider) PrepareVariants(ctx context.Context, req PrepareRequest) (Draft, error) {
	return Draft{
		Mode: "demo",
		Variants: []Variant{
			{ID: "variant_keep", Title: "Сохранить имеющееся", Summary: "Минимальные изменения, акцент на текущие вещи.", SKUs: req.AllowedSKUs},
			{ID: "variant_balance", Title: "Сбалансированное обновление", Summary: "Обновление отделки в рамках бюджета.", SKUs: req.AllowedSKUs},
			{ID: "variant_bold", Title: "Заметное изменение", Summary: "Смена характера выбранного помещения.", SKUs: req.AllowedSKUs},
		},
		Questions: []string{"Подтвердите размеры помещений до покупки"},
	}, nil
}
