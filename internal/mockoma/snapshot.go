package mockoma

import (
	"encoding/json"
)

type snapshot struct {
	Carts     map[string]*Cart
	ByProject map[string]string
	CartSeq   int
	Quotes    map[string]*Quote
	Checkouts map[string]*CheckoutSession
	Orders    map[string]*Order
	OrderSeq  int
}

func (s *Service) Export() (json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return json.Marshal(snapshot{Carts: s.carts, ByProject: s.cartByProject, CartSeq: s.cartSeq, Quotes: s.quotes, Checkouts: s.checkouts, Orders: s.orders, OrderSeq: s.orderSeq})
}
func (s *Service) Import(raw json.RawMessage) error {
	var v snapshot
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.carts = v.Carts
	s.cartByProject = v.ByProject
	s.cartSeq = v.CartSeq
	s.quotes = v.Quotes
	s.checkouts = v.Checkouts
	s.orders = v.Orders
	s.orderSeq = v.OrderSeq
	return nil
}
