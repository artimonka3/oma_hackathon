package httpapi

import (
	"context"
	"math"
	"net/http"
	"oma/internal/apperr"
	"oma/internal/integrations/oma"
	"oma/internal/project"
	"oma/internal/prorab"
	"sort"
	"strconv"
	"strings"
	"time"
)

type ProjectMeta struct {
	Foreman      string `json:"foreman"`
	Checkout     string `json:"checkout"`
	Quote        string `json:"quote"`
	CheckoutHash string `json:"checkout_hash"`
	Order        string `json:"order"`
	Finished     bool   `json:"finished"`
}
type ApplicationState struct {
	Projects map[string]*ProjectMeta `json:"projects"`
}

func (s *Server) meta(pid string) *ProjectMeta {
	v := s.cfg.State.Projects[pid]
	if v == nil {
		v = &ProjectMeta{}
		s.cfg.State.Projects[pid] = v
	}
	return v
}
func (s *Server) owner(r *http.Request, sess Session, pid string) (*project.Project, error) {
	p, e := s.cfg.Projects.GetForActor(r.Context(), project.UserID(sess.UserID), project.ProjectID(pid))
	if e != nil {
		return nil, e
	}
	if string(p.OwnerID) != sess.UserID {
		return nil, apperr.Forbidden("Действие доступно владельцу")
	}
	return p, nil
}
func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request, sess Session) {
	writeJSON(w, 200, map[string]any{"projects": s.cfg.Projects.List(r.Context(), project.UserID(sess.UserID))})
}
func (s *Server) handleCatalog(w http.ResponseWriter, r *http.Request, sess Session) {
	items, e := s.cfg.Oma.SearchProducts(r.Context(), r.URL.Query().Get("q"))
	if e != nil {
		respondError(w, r, e)
		return
	}
	for i := range items {
		items[i] = oma.NormalizeProduct(items[i])
	}
	writeJSON(w, 200, map[string]any{"products": items})
}
func (s *Server) handleApplication(w http.ResponseWriter, r *http.Request, sess Session) {
	pid := r.PathValue("project_id")
	p, e := s.cfg.Projects.GetState(r.Context(), project.UserID(sess.UserID), project.ProjectID(pid))
	if e != nil {
		respondError(w, r, e)
		return
	}
	inventory, _ := s.cfg.Projects.ListInventory(r.Context(), project.UserID(sess.UserID), project.ProjectID(pid))
	spec, _ := s.cfg.Soberi.LatestSpecification(pid)
	sel, _ := s.cfg.Soberi.SelectedDesignOf(pid)
	plans, stages := s.cfg.Prorab.ListWorkPlans(pid)
	reports := map[string][]prorab.Report{}
	for _, st := range stages {
		reports[st.ID] = s.cfg.Prorab.StageReports(st.ID)
	}
	var cart *oma.Cart
	var order *oma.Order
	if s.cfg.Commerce != nil {
		cart, _ = s.cfg.Commerce.CartForProject(pid)
		if m := s.meta(pid); m.Order != "" {
			order, _ = s.cfg.Commerce.GetOrder(m.Order)
		}
	}
	reviews := s.cfg.Prorab.ListReviews(pid)
	for i := range reviews {
		if reviews[i].Status == prorab.ReviewApproved && !s.cfg.Prorab.ValidReview(r.Context(), pid, reviews[i].ForemanID) {
			reviews[i].Status = prorab.ReviewSuperseded
		}
	}
	writeJSON(w, 200, map[string]any{"project": p.Project, "rooms": p.Rooms, "inventory": inventory, "draft": s.cfg.Soberi.DraftOf(pid), "selection": sel, "specification": spec, "cart": cart, "meta": s.meta(pid), "reviews": reviews, "plans": plans, "stages": stages, "reports": reports, "progress": s.cfg.Prorab.AcceptanceProgress(pid), "order": order, "events": s.cfg.Events.ByProject(pid)})
}
func (s *Server) handleRoomPut(w http.ResponseWriter, r *http.Request, sess Session) {
	var in project.AddRoomInput
	if !decodeJSON(w, r, &in) {
		return
	}
	out, e := s.cfg.Projects.PutRoom(r.Context(), project.UserID(sess.UserID), project.ProjectID(r.PathValue("project_id")), in)
	if e != nil {
		respondError(w, r, e)
		return
	}
	writeJSON(w, 200, out)
}
func (s *Server) handleInventoryPut(w http.ResponseWriter, r *http.Request, sess Session) {
	pid := r.PathValue("project_id")
	if _, e := s.owner(r, sess, pid); e != nil {
		respondError(w, r, e)
		return
	}
	var in struct {
		Items []project.InventoryInput `json:"items"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	for _, v := range in.Items {
		amount, e := strconv.ParseFloat(strings.ReplaceAll(v.Amount, ",", "."), 64)
		pr, pe := s.cfg.Oma.GetProduct(r.Context(), v.SKU)
		if e != nil || pe != nil || math.IsNaN(amount) || amount <= 0 || amount > 100000 || v.Unit != pr.Unit {
			respondError(w, r, apperr.InvalidInput("Проверьте товар, количество и единицы остатка", nil))
			return
		}
	}
	if e := s.cfg.Projects.ReplaceInventory(r.Context(), project.UserID(sess.UserID), project.ProjectID(pid), in.Items); e != nil {
		respondError(w, r, e)
		return
	}
	writeJSON(w, 200, map[string]any{"saved": true})
}
func (s *Server) handleSetPacks(w http.ResponseWriter, r *http.Request, sess Session) {
	pid := r.PathValue("project_id")
	if _, e := s.owner(r, sess, pid); e != nil {
		respondError(w, r, e)
		return
	}
	var in struct {
		SKU   string `json:"sku"`
		Packs int    `json:"packs"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if e := s.cfg.Soberi.SetPacks(r.Context(), project.UserID(sess.UserID), pid, in.SKU, in.Packs); e != nil {
		respondError(w, r, e)
		return
	}
	out, e := s.cfg.Soberi.CartSync(r.Context(), project.UserID(sess.UserID), pid)
	if e != nil {
		respondError(w, r, e)
		return
	}
	writeJSON(w, 200, out)
}
func (s *Server) handleAssign(w http.ResponseWriter, r *http.Request, sess Session) {
	pid := r.PathValue("project_id")
	if _, e := s.owner(r, sess, pid); e != nil {
		respondError(w, r, e)
		return
	}
	var in struct {
		Foreman string `json:"foreman_id"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if s.meta(pid).Finished {
		respondError(w, r, apperr.InvalidTransition("Проект завершён"))
		return
	}
	plans, _ := s.cfg.Prorab.ListWorkPlans(pid)
	if len(plans) > 0 && s.meta(pid).Foreman != in.Foreman {
		respondError(w, r, apperr.InvalidTransition("Исполнитель уже закреплён в плане работ"))
		return
	}
	if in.Foreman != "" {
		if _, e := s.cfg.Prorab.GetForeman(in.Foreman); e != nil {
			respondError(w, r, e)
			return
		}
	}
	if e := s.cfg.Projects.Assign(r.Context(), project.UserID(sess.UserID), project.ProjectID(pid), project.UserID(in.Foreman)); e != nil {
		respondError(w, r, e)
		return
	}
	s.meta(pid).Foreman = in.Foreman
	writeJSON(w, 200, s.meta(pid))
}
func (s *Server) handleAnalysis(w http.ResponseWriter, r *http.Request, sess Session) {
	pid := r.PathValue("project_id")
	if _, e := s.cfg.Projects.GetForActor(r.Context(), project.UserID(sess.UserID), project.ProjectID(pid)); e != nil {
		respondError(w, r, e)
		return
	}
	sp, e := s.cfg.Soberi.LatestSpecification(pid)
	if e != nil {
		respondError(w, r, e)
		return
	}
	kinds := map[string]string{}
	for _, line := range sp.NewPurchase {
		p, _ := s.cfg.Oma.GetProduct(r.Context(), line.SKU)
		kind := p.Material
		if kind == "paint" || kind == "primer" {
			kind = "painting"
		}
		if kind != "painting" && kind != "flooring" && kind != "tiling" {
			continue
		}
		kinds[kind] = map[string]string{"painting": "Подготовка и покраска стен", "flooring": "Укладка напольного покрытия", "tiling": "Укладка плитки"}[kind]
	}
	// All materials can be supplied from confirmed inventory; selection still defines the work.
	if len(kinds) == 0 {
		sel, _ := s.cfg.Soberi.SelectedDesignOf(pid)
		for _, sku := range sel.SKUs {
			p, _ := s.cfg.Oma.GetProduct(r.Context(), sku)
			k := p.Material
			if k == "paint" {
				k = "painting"
			}
			kinds[k] = map[string]string{"painting": "Покраска стен", "flooring": "Укладка пола", "tiling": "Укладка плитки"}[k]
		}
	}
	matches := []prorab.ForemanWithReasons{}
	for _, f := range s.cfg.Prorab.ListForemen("", "") {
		ok := true
		reasons := []string{}
		for k, label := range kinds {
			has := false
			for _, x := range f.Foreman.Specialities {
				if x == k {
					has = true
				}
			}
			if !has {
				ok = false
			}
			reasons = append(reasons, label)
		}
		if ok && len(kinds) > 0 {
			f.Reasons = reasons
			matches = append(matches, f)
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		a, b := matches[i].Foreman, matches[j].Foreman
		if len(a.Specialities) != len(b.Specialities) {
			return len(a.Specialities) < len(b.Specialities)
		}
		return a.ExperienceYears > b.ExperienceYears
	})
	writeJSON(w, 200, map[string]any{"works": kinds, "foremen": matches, "mode": "rules", "questions": []string{"Уточните состояние основания и удобные сроки с исполнителем"}})
}
func (s *Server) purchaseGate(ctx context.Context, pid string) error {
	p, e := s.cfg.Projects.InternalProject(ctx, project.ProjectID(pid))
	if e != nil {
		return e
	}
	sp, e := s.cfg.Soberi.LatestSpecification(pid)
	if e != nil {
		return e
	}
	if sp.SourceHash != s.cfg.Soberi.BasisHash(ctx, pid) {
		return apperr.NeedsData("Комплект устарел. Пересчитайте материалы", nil)
	}
	rooms, _ := s.cfg.Projects.RoomsOf(ctx, p.ID)
	if len(rooms) == 0 {
		return apperr.NeedsData("Добавьте размеры", nil)
	}
	for _, r := range rooms {
		if !r.DimensionsConfirmed {
			return apperr.NeedsData("Подтвердите размеры", nil)
		}
	}
	if p.ExecutionMode == project.ModeForeman {
		m := s.meta(pid)
		if m.Foreman == "" || !s.cfg.Prorab.ValidReview(ctx, pid, m.Foreman) {
			return apperr.New("REVIEW_REQUIRED", "Нужна актуальная проверка выбранного прораба", nil)
		}
	}
	return nil
}
func (s *Server) handlePrepareCheckout(w http.ResponseWriter, r *http.Request, sess Session) {
	pid := r.PathValue("project_id")
	if _, e := s.owner(r, sess, pid); e != nil {
		respondError(w, r, e)
		return
	}
	if s.cfg.Commerce == nil {
		respondError(w, r, apperr.IntegrationUnavailable("Запустите приложение со встроенным магазином"))
		return
	}
	if e := s.purchaseGate(r.Context(), pid); e != nil {
		respondError(w, r, e)
		return
	}
	var in struct {
		Confirmed bool `json:"confirmed"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	if !in.Confirmed {
		respondError(w, r, apperr.InvalidInput("Подтвердите состав заказа", nil))
		return
	}
	m := s.meta(pid)
	if m.Order != "" {
		o, _ := s.cfg.Commerce.GetOrder(m.Order)
		writeJSON(w, 200, map[string]any{"order": o})
		return
	}
	// Reuse an unexpired checkout for safe retry and refresh.
	hash := s.cfg.Soberi.ReviewHash(pid)
	if m.Checkout != "" && m.CheckoutHash == hash {
		q, e := s.cfg.Commerce.Quote(m.Quote)
		if e == nil && time.Now().Before(q.ExpiresAt) {
			ch, _ := s.cfg.Commerce.Checkout(m.Checkout)
			writeJSON(w, 200, map[string]any{"checkout": ch, "quote": q})
			return
		}
	}
	if _, e := s.cfg.Soberi.CartSync(r.Context(), project.UserID(sess.UserID), pid); e != nil {
		respondError(w, r, e)
		return
	}
	cart, e := s.cfg.Commerce.CartForProject(pid)
	if e != nil {
		respondError(w, r, e)
		return
	}
	if len(cart.Lines) == 0 {
		respondError(w, r, apperr.NeedsData("Новая закупка не требуется: материалы уже есть", nil))
		return
	}
	q, e := s.cfg.Commerce.CreateQuote(cart.ID)
	if e != nil {
		respondError(w, r, e)
		return
	}
	ch, e := s.cfg.Commerce.CreateCheckout(q.ID)
	if e != nil {
		respondError(w, r, e)
		return
	}
	m.Checkout = ch.ID
	m.Quote = q.ID
	m.CheckoutHash = hash
	writeJSON(w, 201, map[string]any{"checkout": ch, "quote": q})
}
func (s *Server) handlePay(w http.ResponseWriter, r *http.Request, sess Session) {
	pid := r.PathValue("project_id")
	if _, e := s.owner(r, sess, pid); e != nil {
		respondError(w, r, e)
		return
	}
	if s.cfg.Commerce == nil {
		respondError(w, r, apperr.IntegrationUnavailable("Магазин не настроен"))
		return
	}
	m := s.meta(pid)
	if m.Order != "" {
		o, _ := s.cfg.Commerce.GetOrder(m.Order)
		writeJSON(w, 200, o)
		return
	}
	if e := s.purchaseGate(r.Context(), pid); e != nil {
		respondError(w, r, e)
		return
	}
	if m.CheckoutHash != s.cfg.Soberi.ReviewHash(pid) {
		respondError(w, r, apperr.NeedsData("Комплект изменился. Подтвердите заказ заново", nil))
		return
	}
	q, e := s.cfg.Commerce.Quote(m.Quote)
	if e != nil {
		respondError(w, r, e)
		return
	}
	cart, e := s.cfg.Commerce.CartForProject(pid)
	if e != nil || q.Version != cart.Version || q.TotalMinor != cart.TotalMinor {
		respondError(w, r, apperr.New("CART_CHANGED", "Корзина изменилась", nil))
		return
	}
	s.permits.Store(m.Quote, checkoutPermit{ProjectID: pid, CartID: q.CartID, TotalMinor: q.TotalMinor, Currency: q.Currency, Hash: m.CheckoutHash, ExpiresAt: q.ExpiresAt})
	defer s.permits.Delete(m.Quote)
	o, e := s.cfg.Commerce.Pay(r.Context(), m.Checkout)
	if e != nil {
		respondError(w, r, e)
		return
	}
	m.Order = o.ID
	s.cfg.Events.Append(pid, "order", "order.paid", sess.UserID, map[string]any{"order_id": o.ID})
	writeJSON(w, 200, o)
}
func (s *Server) handleDelivery(w http.ResponseWriter, r *http.Request, sess Session) {
	pid := r.PathValue("project_id")
	if _, e := s.owner(r, sess, pid); e != nil {
		respondError(w, r, e)
		return
	}
	m := s.meta(pid)
	if s.cfg.Commerce == nil || m.Order == "" {
		respondError(w, r, apperr.NeedsData("Сначала оформите заказ", nil))
		return
	}
	o, e := s.cfg.Commerce.DemoOrderEvent(r.Context(), m.Order, "deliver", "Демонстрационная доставка")
	if e != nil {
		respondError(w, r, e)
		return
	}
	s.cfg.Events.Append(pid, "order", "order.delivered", sess.UserID, nil)
	writeJSON(w, 200, o)
}
func (s *Server) handleDefaultPlan(w http.ResponseWriter, r *http.Request, sess Session) {
	pid := r.PathValue("project_id")
	if _, e := s.owner(r, sess, pid); e != nil {
		respondError(w, r, e)
		return
	}
	m := s.meta(pid)
	if m.Foreman == "" {
		respondError(w, r, apperr.NeedsData("Выберите прораба", nil))
		return
	}
	if e := s.purchaseGate(r.Context(), pid); e != nil {
		respondError(w, r, e)
		return
	}
	plans, _ := s.cfg.Prorab.ListWorkPlans(pid)
	if len(plans) > 0 {
		writeJSON(w, 200, plans[0].Plan)
		return
	}
	p, e := s.cfg.Prorab.CreateWorkPlan(r.Context(), project.UserID(sess.UserID), pid, prorab.CreatePlanInput{ForemanID: m.Foreman, Stages: []prorab.StageInput{{Title: "Подготовка основания", Weight: 40, Required: true}, {Title: "Отделочные работы", Weight: 60, Required: true}}})
	if e != nil {
		respondError(w, r, e)
		return
	}
	writeJSON(w, 201, p)
}
func (s *Server) handleFinish(w http.ResponseWriter, r *http.Request, sess Session) {
	pid := r.PathValue("project_id")
	if _, e := s.owner(r, sess, pid); e != nil {
		respondError(w, r, e)
		return
	}
	if s.cfg.Prorab.AcceptanceProgress(pid) != 100 {
		respondError(w, r, apperr.NeedsData("Сначала примите все обязательные этапы", nil))
		return
	}
	m := s.meta(pid)
	if !m.Finished {
		s.cfg.Events.Append(pid, "project", "project.completed", sess.UserID, nil)
		m.Finished = true
	}
	writeJSON(w, 200, m)
}
func (s *Server) readyToStart(ctx context.Context, pid, stageID string) error {
	_, stages := s.cfg.Prorab.ListWorkPlans(pid)
	for _, st := range stages {
		if st.ID == stageID {
			break
		}
		if st.Required && st.Status != prorab.StageAccepted {
			return apperr.NeedsData("Сначала примите предыдущий этап", nil)
		}
	}
	sp, e := s.cfg.Soberi.LatestSpecification(pid)
	if e != nil {
		return e
	}
	if len(sp.NewPurchase) > 0 {
		if s.cfg.Commerce == nil {
			return apperr.NeedsData("Нет данных о доставке", nil)
		}
		m := s.meta(pid)
		o, e := s.cfg.Commerce.GetOrder(m.Order)
		if e != nil || o.Status != "delivered" {
			return apperr.NeedsData("Дождитесь доставки материалов", nil)
		}
	}
	return nil
}

func (s *Server) handleCatalogProduct(w http.ResponseWriter, r *http.Request, sess Session) {
	p, e := s.cfg.Oma.GetProduct(r.Context(), r.PathValue("sku"))
	if e != nil {
		respondError(w, r, e)
		return
	}
	writeJSON(w, 200, oma.NormalizeProduct(p))
}
