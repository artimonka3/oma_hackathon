package soberi

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"

	"oma/internal/ai"
	"oma/internal/apperr"
	"oma/internal/events"
	"oma/internal/id"
	"oma/internal/integrations/oma"
	"oma/internal/jobs"
	"oma/internal/project"
)

// Service — бизнес-логика ОМА.Собери.
type Service struct {
	projects *project.Service
	oma      oma.Client
	queue    *jobs.Queue
	provider ai.Provider
	events   *events.Log

	mu         sync.Mutex
	drafts     map[string]ai.Draft       // project_id -> черновик
	selections map[string]SelectedDesign // project_id -> выбранная версия
	specs      map[string]*Specification
	specVers   map[string]int64
}

// NewService собирает домен Собери на границах: проекты, магазин, очередь,
// AI-провайдер и журнал событий.
func NewService(p *project.Service, shop oma.Client, q *jobs.Queue, provider ai.Provider, ev *events.Log) *Service {
	return &Service{
		projects:   p,
		oma:        shop,
		queue:      q,
		provider:   provider,
		events:     ev,
		drafts:     map[string]ai.Draft{},
		selections: map[string]SelectedDesign{},
		specs:      map[string]*Specification{},
		specVers:   map[string]int64{},
	}
}

// StartDesignJob ставит задание подготовки вариантов. Разрешённый каталог
// передаётся модели явно; AI не придумывает артикулы сам.
func (s *Service) StartDesignJob(ctx context.Context, actor project.UserID, pid string) (*jobs.Job, error) {
	p, err := s.projects.GetForActor(ctx, actor, project.ProjectID(pid))
	if err != nil {
		return nil, err
	}
	rooms, err := s.projects.RoomsOf(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	products, err := s.oma.SearchProducts(ctx, "")
	if err != nil {
		return nil, err
	}
	if len(products) == 0 {
		return nil, apperr.NeedsData("каталог магазина недоступен или пуст", []string{"Добавьте товары в каталог"})
	}
	allowed := make([]string, 0, len(products))
	roomNames := make([]string, 0, len(rooms))
	for _, pr := range products {
		allowed = append(allowed, pr.SKU)
	}
	for _, r := range rooms {
		roomNames = append(roomNames, r.Name)
	}
	req := ai.PrepareRequest{
		ProjectID:   pid,
		Goal:        p.Goal,
		RoomNames:   roomNames,
		AllowedSKUs: allowed,
	}
	job := s.queue.Enqueue("design_variants", pid, req)
	s.events.Append(pid, "design", "design.job.queued", string(actor), map[string]any{"job_id": job.ID})
	return job, nil
}

// DesignJobResult — результат выполненного задания.
type DesignJobResult struct {
	Variants  []ai.Variant `json:"variants"`
	Questions []string     `json:"questions"`
	Mode      string       `json:"mode"`
}

// HandleDesignJob — обработчик задания очереди: вызывает AI-провайдер,
// валидирует ответ и сохраняет черновик. Вызывается worker'ом, не из HTTP.
func (s *Service) HandleDesignJob(ctx context.Context, job *jobs.Job) (any, error) {
	req, ok := job.Input.(ai.PrepareRequest)
	if !ok {
		return nil, apperr.InvalidInput("задание содержит некорректный вход", nil)
	}
	draft, err := s.provider.PrepareVariants(ctx, req)
	if err != nil {
		return nil, err
	}
	// Валидация ответа модели: черновик обязан содержать варианты
	// и не выходить за разрешённый каталог (раздел 8 ТЗ).
	allowed := map[string]bool{}
	for _, sku := range req.AllowedSKUs {
		allowed[sku] = true
	}
	for _, v := range draft.Variants {
		for _, sku := range v.SKUs {
			if !allowed[sku] {
				return nil, apperr.InvalidInput("AI предложил артикул вне разрешённого каталога: "+sku, nil)
			}
		}
	}
	if len(draft.Variants) == 0 {
		return nil, apperr.InvalidInput("AI не предложил ни одного варианта", nil)
	}
	s.mu.Lock()
	s.drafts[req.ProjectID] = draft
	s.mu.Unlock()
	s.events.Append(req.ProjectID, "design", "design.draft.ready", "worker", map[string]any{"job_id": job.ID, "mode": draft.Mode})
	return DesignJobResult{Variants: draft.Variants, Questions: draft.Questions, Mode: draft.Mode}, nil
}

// SelectDesign подтверждает выбранную версию проекта.
func (s *Service) SelectDesign(ctx context.Context, actor project.UserID, pid, variantID string) (*SelectedDesign, error) {
	if _, err := s.projects.GetForActor(ctx, actor, project.ProjectID(pid)); err != nil {
		return nil, err
	}
	s.mu.Lock()
	draft, ok := s.drafts[pid]
	s.mu.Unlock()
	if !ok {
		return nil, apperr.NeedsData("черновик вариантов ещё не готов", []string{"Запустите подготовку вариантов"})
	}
	for _, v := range draft.Variants {
		if v.ID == variantID {
			sel := SelectedDesign{
				ID:        id.New("sel"),
				ProjectID: pid,
				VariantID: v.ID,
				Title:     v.Title,
				SKUs:      v.SKUs,
				Author:    string(actor),
			}
			s.mu.Lock()
			s.selections[pid] = sel
			s.mu.Unlock()
			s.events.Append(pid, "design", "design.selected", string(actor), map[string]any{"variant_id": v.ID})
			return &sel, nil
		}
	}
	return nil, apperr.NotFound("выбранный вариант не найден в черновике")
}

// SelectedDesignOf возвращает выбранную версию (используется другими модулями).
func (s *Service) SelectedDesignOf(pid string) (SelectedDesign, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sel, ok := s.selections[pid]
	return sel, ok
}

// CalculateSpecification считает состав по геометрии и правилам категории.
// Подтверждённые размеры обязательны: иначе возвращается NEEDS_DATA
// со списком вопросов.
func (s *Service) CalculateSpecification(ctx context.Context, actor project.UserID, pid string) (*Specification, error) {
	p, err := s.projects.GetForActor(ctx, actor, project.ProjectID(pid))
	if err != nil {
		return nil, err
	}
	rooms, err := s.projects.RoomsOf(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	var questions []string
	if len(rooms) == 0 {
		questions = append(questions, "Добавьте хотя бы одно помещение")
	}
	for _, r := range rooms {
		if !r.DimensionsConfirmed {
			questions = append(questions, "Подтвердите размеры помещения "+r.Name)
		}
	}
	if len(questions) > 0 {
		return nil, apperr.NeedsData("спецификация не может быть рассчитана", questions)
	}
	products, err := s.oma.SearchProducts(ctx, paintCategory)
	if err != nil {
		return nil, err
	}
	var paint *oma.Product
	for i := range products {
		if products[i].Stock > 0 && (paint == nil || products[i].PriceMinor < paint.PriceMinor) {
			pr := products[i]
			paint = &pr
		}
	}
	if paint == nil {
		return nil, apperr.NeedsData("в каталоге нет доступной краски", []string{"Добавьте товары категории paint"})
	}
	needs := make([]RoomNeed, 0, len(rooms))
	byRoom := map[string]float64{}
	for _, r := range rooms {
		wallArea := 2 * float64(r.WidthMM+r.LengthMM) * float64(r.HeightMM) / 1_000_000
		liters := wallArea * paintLitersPerM2
		needs = append(needs, RoomNeed{
			RoomID:      string(r.ID),
			Name:        r.Name,
			WallAreaM2:  math.Round(wallArea*100) / 100,
			PaintLiters: math.Round(liters*100) / 100,
		})
		byRoom[string(r.ID)] = liters
	}
	// Агрегация упаковок по артикулу с сохранением распределения по комнатам.
	packs := 0
	var roomIDs []string
	for _, r := range rooms {
		l := byRoom[string(r.ID)]
		roomPacks := int(math.Ceil(l / float64(paint.PackAmount)))
		packs += roomPacks
		roomIDs = append(roomIDs, string(r.ID))
	}
	sort.Strings(roomIDs)
	spec := &Specification{
		ID:        id.New("spec"),
		ProjectID: pid,
		RoomNeeds: needs,
		NewPurchase: []PurchaseLine{{
			SKU:        paint.SKU,
			Title:      paint.Title,
			Packs:      packs,
			PriceMinor: paint.PriceMinor,
			TotalMinor: int64(packs) * paint.PriceMinor,
			RoomIDs:    roomIDs,
		}},
		TotalMinor:     int64(packs) * paint.PriceMinor,
		Currency:       paint.Currency,
		RuleSetVersion: ruleSetVersion,
		Demo:           paint.Demo,
	}
	if sel, ok := s.SelectedDesignOf(pid); ok {
		spec.Questions = append(spec.Questions, "Проверьте состав относительно выбранного варианта "+sel.Title)
	}
	s.mu.Lock()
	s.specVers[pid]++
	spec.Version = s.specVers[pid]
	s.specs[pid] = spec
	s.mu.Unlock()
	s.events.Append(pid, "specification", "specification.calculated", string(actor),
		map[string]any{"version": spec.Version, "total_minor": spec.TotalMinor, "rule_set_version": ruleSetVersion})
	return spec, nil
}

// LatestSpecification возвращает последнюю версию состава проекта.
func (s *Service) LatestSpecification(pid string) (*Specification, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	spec, ok := s.specs[pid]
	if !ok {
		return nil, apperr.NotFound("спецификация ещё не рассчитана")
	}
	cp := *spec
	return &cp, nil
}

// CartSync создаёт/обновляет связанные строки торговой корзины.
// Идемпотентность обеспечивается связями строк: повторный переход
// обновляет строки проекта, а не удваивает покупку.
func (s *Service) CartSync(ctx context.Context, actor project.UserID, pid string) (oma.SyncResult, error) {
	if _, err := s.projects.GetForActor(ctx, actor, project.ProjectID(pid)); err != nil {
		return oma.SyncResult{}, err
	}
	spec, err := s.LatestSpecification(pid)
	if err != nil {
		return oma.SyncResult{}, apperr.NeedsData("сначала рассчитайте спецификацию", []string{"POST /api/v1/projects/{id}/specifications"})
	}
	lines := make([]oma.SyncLine, 0, len(spec.NewPurchase))
	for _, line := range spec.NewPurchase {
		roomID := ""
		if len(line.RoomIDs) > 0 {
			roomID = line.RoomIDs[0]
		}
		lines = append(lines, oma.SyncLine{
			LinkID: fmt.Sprintf("%s:%s", pid, line.SKU),
			SKU:    line.SKU,
			Packs:  line.Packs,
			RoomID: roomID,
		})
	}
	res, err := s.oma.SyncCart(ctx, oma.SyncRequest{ProjectID: pid, Lines: lines})
	if err != nil {
		return oma.SyncResult{}, err
	}
	s.events.Append(pid, "cart", "cart.synced", string(actor), map[string]any{
		"cart_id": res.CartID, "cart_version": res.CartVersion, "upserted": res.Upserted,
	})
	return res, nil
}
