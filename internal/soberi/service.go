package soberi

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
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
	lots, _ := s.projects.ListInventory(ctx, actor, p.ID)
	available := []oma.Product{}
	for _, product := range products {
		product = oma.NormalizeProduct(product)
		need := 0.0
		for _, room := range rooms {
			if product.Material == "paint" {
				need += 2 * float64(room.WidthMM+room.LengthMM) * float64(room.HeightMM) / 1e6 * product.Coverage * 2
			} else {
				need += float64(room.WidthMM) * float64(room.LengthMM) / 1e6 * 1.1
			}
		}
		for _, lot := range lots {
			if lot.Status == project.LotConfirmed && lot.SKU == product.SKU && lot.Unit == product.Unit {
				n, _ := strconv.ParseFloat(strings.ReplaceAll(lot.Amount, ",", "."), 64)
				need -= n
			}
		}
		if product.CalculationSupported && math.Ceil(math.Max(0, need)/product.PackAmount-1e-9) <= float64(product.Stock) {
			available = append(available, product)
		}
	}
	products = RecommendationProducts(p.Goal, available)
	if len(products) == 0 {
		return nil, apperr.NeedsData("Нет подходящих материалов в нужном количестве", []string{"Уточните задачу, размеры или доступность материалов в магазине"})
	}
	allowed := make([]string, 0, len(products))
	roomNames := make([]string, 0, len(rooms))
	items := []ai.CatalogItem{}
	for _, pr := range products {
		pr = oma.NormalizeProduct(pr)
		if !pr.CalculationSupported {
			continue
		}
		allowed = append(allowed, pr.SKU)
		items = append(items, ai.CatalogItem{SKU: pr.SKU, Title: pr.Title, Material: pr.Material, Unit: pr.Unit, PackAmount: pr.PackAmount, PriceMinor: pr.PriceMinor})
	}
	for _, r := range rooms {
		roomNames = append(roomNames, r.Name)
	}
	geometries := []ai.RoomGeometry{}
	for _, room := range rooms {
		geometries = append(geometries, ai.RoomGeometry{Name: room.Name, LengthMM: room.LengthMM, WidthMM: room.WidthMM, HeightMM: room.HeightMM, Confirmed: room.DimensionsConfirmed})
	}
	facts := []ai.InventoryFact{}
	for _, lot := range lots {
		if lot.Status == project.LotConfirmed {
			facts = append(facts, ai.InventoryFact{SKU: lot.SKU, Amount: lot.Amount, Unit: lot.Unit})
		}
	}
	req := ai.PrepareRequest{
		ProjectID:   pid,
		Goal:        p.Goal,
		RoomNames:   roomNames,
		AllowedSKUs: allowed,
		Products:    items,
		Rooms:       geometries, Inventory: facts,
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
		raw, _ := json.Marshal(job.Input)
		if json.Unmarshal(raw, &req) != nil {
			return nil, apperr.InvalidInput("задание содержит некорректный вход", nil)
		}
	}
	draft, err := s.provider.PrepareVariants(ctx, req)
	if err != nil {
		return nil, err
	}
	if err := ai.ValidateDraft(draft, req.AllowedSKUs); err != nil {
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

	sel, chosen := s.SelectedDesignOf(pid)
	if !chosen || len(sel.SKUs) == 0 {
		return nil, apperr.NeedsData("Выберите вариант материалов", []string{"Выберите готовый вариант"})
	}
	lots, _ := s.projects.ListInventory(ctx, actor, project.ProjectID(pid))
	spec := &Specification{ID: id.New("spec"), ProjectID: pid, Currency: "BYN", RuleSetVersion: "v2", Demo: true, RequiredPacks: map[string]int{}}
	for _, sku := range sel.SKUs {
		product, err := s.oma.GetProduct(ctx, sku)
		if err != nil {
			return nil, err
		}
		product = oma.NormalizeProduct(product)
		if !product.CalculationSupported {
			return nil, apperr.NeedsData("Для этого товара не хватает данных для расчёта", []string{"Уточните единицы продажи и назначение материала"})
		}
		if product.PackAmount <= 0 {
			return nil, apperr.InvalidInput("Не задан объём упаковки", nil)
		}
		var need float64
		roomIDs := []string{}
		for _, room := range rooms {
			area := float64(room.WidthMM) * float64(room.LengthMM) / 1e6
			walls := 2 * float64(room.WidthMM+room.LengthMM) * float64(room.HeightMM) / 1e6
			roomIDs = append(roomIDs, string(room.ID))
			if product.Material == "paint" {
				need += walls * product.Coverage * 2
				amount := math.Round(walls*product.Coverage*2*100) / 100
				liters := 0.0
				if product.Unit == "l" {
					liters = amount
				}
				spec.RoomNeeds = append(spec.RoomNeeds, RoomNeed{RoomID: string(room.ID), Name: room.Name, WallAreaM2: math.Round(walls*100) / 100, PaintLiters: liters, PaintAmount: amount, PaintUnit: product.Unit})
			} else {
				need += area * 1.1
			}
		}
		used := 0.0
		for _, lot := range lots {
			if lot.Status == project.LotConfirmed && lot.SKU == sku && lot.Unit == product.Unit {
				n, e := strconv.ParseFloat(strings.ReplaceAll(lot.Amount, ",", "."), 64)
				if e == nil && n > 0 {
					used += n
				}
			}
		}
		used = math.Min(need, used)
		spec.UsedInventory += used
		packs := int(math.Ceil(math.Max(0, need-used)/product.PackAmount - 1e-9))
		if packs > product.Stock {
			return nil, apperr.NeedsData("Недостаточно товара на складе", []string{product.Title})
		}
		spec.RequiredPacks[sku] = packs
		if packs > 0 {
			spec.NewPurchase = append(spec.NewPurchase, PurchaseLine{SKU: sku, Title: product.Title, Packs: packs, PriceMinor: product.PriceMinor, TotalMinor: int64(packs) * product.PriceMinor, RoomIDs: roomIDs})
			spec.TotalMinor += int64(packs) * product.PriceMinor
		}
	}
	spec.SourceHash = s.BasisHash(ctx, pid)
	s.mu.Lock()
	s.specVers[pid]++
	spec.Version = s.specVers[pid]
	if old := s.specs[pid]; old != nil {
		for _, line := range old.NewPurchase {
			if line.Manual {
				duplicate := false
				for _, v := range spec.NewPurchase {
					if v.SKU == line.SKU {
						duplicate = true
					}
				}
				if !duplicate {
					spec.NewPurchase = append(spec.NewPurchase, line)
					spec.RequiredPacks[line.SKU] = 1
					spec.TotalMinor += line.TotalMinor
				}
			}
		}
	}
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
	if spec.SourceHash != s.BasisHash(ctx, pid) {
		return oma.SyncResult{}, apperr.NeedsData("Пересчитайте комплект после изменения исходных данных", nil)
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

func (s *Service) DraftOf(pid string) ai.Draft {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.drafts[pid]
}
func (s *Service) BasisHash(ctx context.Context, pid string) string {
	p, err := s.projects.InternalProject(ctx, project.ProjectID(pid))
	if err != nil {
		return ""
	}
	rooms, _ := s.projects.RoomsOf(ctx, p.ID)
	sel, _ := s.SelectedDesignOf(pid)
	raw, _ := json.Marshal([]any{p.Goal, rooms, sel.VariantID, sel.SKUs, s.projects.MaterialInput(ctx, p.ID)})
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}
func (s *Service) ReviewHash(pid string) string {
	sp, err := s.LatestSpecification(pid)
	if err != nil {
		return s.BasisHash(context.Background(), pid)
	}
	raw, _ := json.Marshal(sp.NewPurchase)
	return s.BasisHash(context.Background(), pid) + string(raw)
}
func (s *Service) SetPacks(ctx context.Context, actor project.UserID, pid, sku string, packs int) error {
	if _, err := s.projects.GetForActor(ctx, actor, project.ProjectID(pid)); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sp, ok := s.specs[pid]
	if !ok {
		return apperr.NotFound("Комплект не рассчитан")
	}
	if packs < sp.RequiredPacks[sku] || packs > 999 {
		return apperr.InvalidInput("Количество не может быть меньше расчётного", nil)
	}
	found := false
	for i := range sp.NewPurchase {
		if sp.NewPurchase[i].SKU == sku {
			pr, e := s.oma.GetProduct(ctx, sku)
			if e != nil {
				return e
			}
			if packs > pr.Stock {
				return apperr.InvalidInput("Недостаточно товара на складе", nil)
			}
			sp.NewPurchase[i].Packs = packs
			sp.NewPurchase[i].TotalMinor = int64(packs) * pr.PriceMinor
			found = true
		}
	}
	if !found {
		pr, e := s.oma.GetProduct(ctx, sku)
		if e != nil {
			return e
		}
		if packs < 1 || packs > pr.Stock || pr.PriceMinor <= 0 {
			return apperr.InvalidInput("Проверьте количество и наличие товара", nil)
		}
		sp.NewPurchase = append(sp.NewPurchase, PurchaseLine{SKU: sku, Title: pr.Title, Packs: packs, PriceMinor: pr.PriceMinor, TotalMinor: int64(packs) * pr.PriceMinor, Manual: true})
		sp.RequiredPacks[sku] = 1
	}
	sp.TotalMinor = 0
	for _, l := range sp.NewPurchase {
		sp.TotalMinor += l.TotalMinor
	}
	s.specVers[pid]++
	sp.Version = s.specVers[pid]
	return nil
}
