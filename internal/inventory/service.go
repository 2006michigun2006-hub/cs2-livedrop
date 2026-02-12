package inventory

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/2006michigun2006-hub/cs2-livedrop/internal/wallet"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Item struct {
	ID         int64           `json:"id"`
	UserID     int64           `json:"user_id"`
	ItemType   string          `json:"item_type"`
	Name       string          `json:"name"`
	Rarity     string          `json:"rarity"`
	PriceCents int64           `json:"price_cents"`
	Status     string          `json:"status"`
	Source     string          `json:"source"`
	ParentItem *int64          `json:"parent_item_id,omitempty"`
	Metadata   json.RawMessage `json:"metadata"`
	CreatedAt  time.Time       `json:"created_at"`
	OpenedAt   *time.Time      `json:"opened_at,omitempty"`
	SoldAt     *time.Time      `json:"sold_at,omitempty"`
}

type Service struct {
	db      *pgxpool.Pool
	wallet  *wallet.Service
	pricing *priceResolver
}

type weightedDrop struct {
	Name   string
	Rarity string
	Weight int64
}

func NewService(db *pgxpool.Pool, walletService *wallet.Service) *Service {
	return &Service{
		db:      db,
		wallet:  walletService,
		pricing: newPriceResolver(),
	}
}

func (s *Service) GrantItem(ctx context.Context, userID int64, itemType, name, rarity, source string, metadata map[string]interface{}) (Item, error) {
	itemType = strings.ToLower(strings.TrimSpace(itemType))
	if itemType != "skin" && itemType != "case" {
		return Item{}, errors.New("invalid item_type")
	}
	if strings.TrimSpace(name) == "" {
		return Item{}, errors.New("name is required")
	}
	if rarity == "" {
		rarity = "consumer"
	}
	if source == "" {
		source = "system"
	}
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	priceCents := extractPriceCents(metadata)
	if priceCents <= 0 {
		priceCents = s.resolveItemPrice(ctx, itemType, name, rarity, 0)
	}
	metadata["price_cents"] = priceCents

	rawMeta, err := json.Marshal(metadata)
	if err != nil {
		return Item{}, err
	}

	status := "available"
	if itemType == "case" {
		status = "unopened"
	}

	var item Item
	err = s.db.QueryRow(ctx, `
INSERT INTO inventory_items (user_id, item_type, name, rarity, price_cents, status, source, metadata)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, user_id, item_type, name, rarity, price_cents, status, source, parent_item_id, metadata, created_at, opened_at, sold_at
`, userID, itemType, name, rarity, priceCents, status, source, rawMeta).Scan(
		&item.ID,
		&item.UserID,
		&item.ItemType,
		&item.Name,
		&item.Rarity,
		&item.PriceCents,
		&item.Status,
		&item.Source,
		&item.ParentItem,
		&item.Metadata,
		&item.CreatedAt,
		&item.OpenedAt,
		&item.SoldAt,
	)
	return item, err
}

func (s *Service) ListByUser(ctx context.Context, userID int64, limit int) ([]Item, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}

	rows, err := s.db.Query(ctx, `
SELECT id, user_id, item_type, name, rarity, price_cents, status, source, parent_item_id, metadata, created_at, opened_at, sold_at
FROM inventory_items
WHERE user_id = $1
  AND status <> 'sold'
  AND NOT (item_type = 'case' AND status = 'opened')
ORDER BY created_at DESC
LIMIT $2
`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]Item, 0)
	for rows.Next() {
		var item Item
		if err := rows.Scan(&item.ID, &item.UserID, &item.ItemType, &item.Name, &item.Rarity, &item.PriceCents, &item.Status, &item.Source, &item.ParentItem, &item.Metadata, &item.CreatedAt, &item.OpenedAt, &item.SoldAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) OpenCase(ctx context.Context, userID, caseItemID int64) (Item, Item, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Item{}, Item{}, err
	}
	defer tx.Rollback(ctx)

	var caseItem Item
	err = tx.QueryRow(ctx, `
SELECT id, user_id, item_type, name, rarity, price_cents, status, source, parent_item_id, metadata, created_at, opened_at, sold_at
FROM inventory_items
WHERE id = $1 AND user_id = $2
FOR UPDATE
`, caseItemID, userID).Scan(
		&caseItem.ID,
		&caseItem.UserID,
		&caseItem.ItemType,
		&caseItem.Name,
		&caseItem.Rarity,
		&caseItem.PriceCents,
		&caseItem.Status,
		&caseItem.Source,
		&caseItem.ParentItem,
		&caseItem.Metadata,
		&caseItem.CreatedAt,
		&caseItem.OpenedAt,
		&caseItem.SoldAt,
	)
	if err != nil {
		return Item{}, Item{}, err
	}
	if caseItem.ItemType != "case" || caseItem.Status != "unopened" {
		return Item{}, Item{}, errors.New("item is not an unopened case")
	}

	drop, err := chooseDrop(dropPoolForCase(caseItem.Name))
	if err != nil {
		return Item{}, Item{}, err
	}
	dropPrice := s.resolveItemPrice(ctx, "skin", drop.Name, drop.Rarity, 0)
	dropMeta, _ := json.Marshal(map[string]interface{}{
		"from_case_id":   caseItemID,
		"from_case_name": caseItem.Name,
		"price_cents":    dropPrice,
	})

	var skin Item
	err = tx.QueryRow(ctx, `
INSERT INTO inventory_items (user_id, item_type, name, rarity, price_cents, status, source, parent_item_id, metadata)
VALUES ($1, 'skin', $2, $3, $4, 'available', 'case_open', $5, $6)
RETURNING id, user_id, item_type, name, rarity, price_cents, status, source, parent_item_id, metadata, created_at, opened_at, sold_at
`, userID, drop.Name, drop.Rarity, dropPrice, caseItemID, dropMeta).Scan(
		&skin.ID,
		&skin.UserID,
		&skin.ItemType,
		&skin.Name,
		&skin.Rarity,
		&skin.PriceCents,
		&skin.Status,
		&skin.Source,
		&skin.ParentItem,
		&skin.Metadata,
		&skin.CreatedAt,
		&skin.OpenedAt,
		&skin.SoldAt,
	)
	if err != nil {
		return Item{}, Item{}, err
	}

	openedMeta, _ := json.Marshal(map[string]interface{}{"opened_item_id": skin.ID, "opened_item_name": skin.Name, "opened_item_rarity": skin.Rarity})
	_, err = tx.Exec(ctx, `
UPDATE inventory_items
SET status = 'opened', opened_at = NOW(), metadata = $2
WHERE id = $1
`, caseItemID, openedMeta)
	if err != nil {
		return Item{}, Item{}, err
	}

	now := time.Now().UTC()
	caseItem.Status = "opened"
	caseItem.OpenedAt = &now
	caseItem.Metadata = openedMeta

	if err := tx.Commit(ctx); err != nil {
		return Item{}, Item{}, err
	}
	return caseItem, skin, nil
}

func defaultDropPool() []weightedDrop {
	return []weightedDrop{
		{Name: "P250 | Sand Dune", Rarity: "consumer", Weight: 35},
		{Name: "MP9 | Storm", Rarity: "industrial", Weight: 25},
		{Name: "UMP-45 | Briefing", Rarity: "mil-spec", Weight: 18},
		{Name: "AK-47 | Slate", Rarity: "restricted", Weight: 12},
		{Name: "M4A1-S | Cyrex", Rarity: "classified", Weight: 7},
		{Name: "AWP | Wildfire", Rarity: "covert", Weight: 3},
	}
}

func knifePremiumDropPool() []weightedDrop {
	// Premium knife case: better chances for expensive tiers versus default pool.
	return []weightedDrop{
		{Name: "P250 | Sand Dune", Rarity: "consumer", Weight: 18},
		{Name: "MP9 | Storm", Rarity: "industrial", Weight: 16},
		{Name: "UMP-45 | Briefing", Rarity: "mil-spec", Weight: 18},
		{Name: "AK-47 | Slate", Rarity: "restricted", Weight: 18},
		{Name: "M4A1-S | Cyrex", Rarity: "classified", Weight: 14},
		{Name: "AWP | Wildfire", Rarity: "covert", Weight: 11},
		{Name: "Karambit | Doppler", Rarity: "gold", Weight: 2},
		{Name: "M9 Bayonet | Fade", Rarity: "gold", Weight: 2},
		{Name: "Butterfly Knife | Slaughter", Rarity: "gold", Weight: 1},
	}
}

func dropPoolForCase(caseName string) []weightedDrop {
	name := strings.ToLower(strings.TrimSpace(caseName))
	if strings.Contains(name, "knife") || strings.Contains(name, "premium") || strings.Contains(name, "omega") {
		return knifePremiumDropPool()
	}
	return defaultDropPool()
}

func chooseDrop(pool []weightedDrop) (weightedDrop, error) {
	total := int64(0)
	for _, it := range pool {
		total += it.Weight
	}
	if total <= 0 {
		return weightedDrop{}, errors.New("invalid drop pool")
	}

	r, err := rand.Int(rand.Reader, big.NewInt(total))
	if err != nil {
		return weightedDrop{}, err
	}
	n := r.Int64()

	acc := int64(0)
	for _, item := range pool {
		acc += item.Weight
		if n < acc {
			return item, nil
		}
	}
	return pool[len(pool)-1], nil
}

func (s *Service) GrantItemTx(ctx context.Context, tx pgx.Tx, userID int64, itemType, name, rarity, source string, parentItemID *int64, metadata map[string]interface{}) (Item, error) {
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	priceCents := extractPriceCents(metadata)
	if priceCents <= 0 {
		priceCents = s.resolveItemPrice(ctx, itemType, name, rarity, 0)
	}
	metadata["price_cents"] = priceCents
	rawMeta, err := json.Marshal(metadata)
	if err != nil {
		return Item{}, err
	}
	status := "available"
	if itemType == "case" {
		status = "unopened"
	}
	var item Item
	err = tx.QueryRow(ctx, `
INSERT INTO inventory_items (user_id, item_type, name, rarity, price_cents, status, source, parent_item_id, metadata)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id, user_id, item_type, name, rarity, price_cents, status, source, parent_item_id, metadata, created_at, opened_at, sold_at
`, userID, itemType, name, rarity, priceCents, status, source, parentItemID, rawMeta).Scan(
		&item.ID,
		&item.UserID,
		&item.ItemType,
		&item.Name,
		&item.Rarity,
		&item.PriceCents,
		&item.Status,
		&item.Source,
		&item.ParentItem,
		&item.Metadata,
		&item.CreatedAt,
		&item.OpenedAt,
		&item.SoldAt,
	)
	return item, err
}

func (s *Service) SellItem(ctx context.Context, userID, itemID int64) (Item, int64, error) {
	if s.wallet == nil {
		return Item{}, 0, errors.New("wallet service is not configured")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Item{}, 0, err
	}
	defer tx.Rollback(ctx)

	var item Item
	err = tx.QueryRow(ctx, `
SELECT id, user_id, item_type, name, rarity, price_cents, status, source, parent_item_id, metadata, created_at, opened_at, sold_at
FROM inventory_items
WHERE id = $1 AND user_id = $2
FOR UPDATE
`, itemID, userID).Scan(
		&item.ID,
		&item.UserID,
		&item.ItemType,
		&item.Name,
		&item.Rarity,
		&item.PriceCents,
		&item.Status,
		&item.Source,
		&item.ParentItem,
		&item.Metadata,
		&item.CreatedAt,
		&item.OpenedAt,
		&item.SoldAt,
	)
	if err != nil {
		return Item{}, 0, err
	}

	switch item.Status {
	case "sold":
		return Item{}, 0, errors.New("item already sold")
	case "opened":
		return Item{}, 0, errors.New("opened case cannot be sold")
	}

	saleAmount := item.PriceCents
	if saleAmount <= 0 {
		saleAmount = s.resolveItemPrice(ctx, item.ItemType, item.Name, item.Rarity, 0)
	}
	if saleAmount <= 0 {
		return Item{}, 0, errors.New("item has no market price")
	}

	metadataMap := map[string]interface{}{}
	if len(item.Metadata) > 0 {
		_ = json.Unmarshal(item.Metadata, &metadataMap)
	}
	metadataMap["price_cents"] = saleAmount
	metadataMap["sale_cents"] = saleAmount
	metadataMap["sold_reason"] = "inventory_sell"
	rawMeta, err := json.Marshal(metadataMap)
	if err != nil {
		return Item{}, 0, err
	}

	var soldAt time.Time
	err = tx.QueryRow(ctx, `
UPDATE inventory_items
SET status = 'sold', sold_at = NOW(), price_cents = $2, metadata = $3
WHERE id = $1
RETURNING sold_at
`, itemID, saleAmount, rawMeta).Scan(&soldAt)
	if err != nil {
		return Item{}, 0, err
	}

	newBalance, err := s.wallet.AdjustBalance(ctx, tx, userID, saleAmount, "inventory_sell", map[string]interface{}{
		"item_id":    itemID,
		"item_name":  item.Name,
		"item_type":  item.ItemType,
		"sale_cents": saleAmount,
		"source":     item.Source,
		"rarity":     item.Rarity,
	})
	if err != nil {
		return Item{}, 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Item{}, 0, err
	}

	item.Status = "sold"
	item.SoldAt = &soldAt
	item.PriceCents = saleAmount
	item.Metadata = rawMeta
	return item, newBalance, nil
}

type priceResolver struct {
	client *http.Client
	mu     sync.RWMutex
	cache  map[string]cachedPrice
}

type cachedPrice struct {
	priceCents int64
	expiresAt  time.Time
}

func newPriceResolver() *priceResolver {
	return &priceResolver{
		client: &http.Client{Timeout: 3500 * time.Millisecond},
		cache:  make(map[string]cachedPrice),
	}
}

func (s *Service) resolveItemPrice(ctx context.Context, itemType, name, rarity string, defaultCents int64) int64 {
	if s.pricing == nil {
		return maxPrice(defaultCents, fallbackByRarity(rarity, itemType))
	}
	if p := s.pricing.resolve(ctx, itemType, name, rarity); p > 0 {
		return p
	}
	return maxPrice(defaultCents, fallbackByRarity(rarity, itemType))
}

func (r *priceResolver) resolve(ctx context.Context, itemType, name, rarity string) int64 {
	key := strings.ToLower(strings.TrimSpace(itemType + "|" + name))
	if key == "|" {
		return 0
	}

	r.mu.RLock()
	if cached, ok := r.cache[key]; ok && time.Now().Before(cached.expiresAt) {
		r.mu.RUnlock()
		return cached.priceCents
	}
	r.mu.RUnlock()

	if p := staticKnownPrice(name); p > 0 {
		r.setCache(key, p)
		return p
	}

	if p := r.fetchSteamPrice(ctx, name); p > 0 {
		r.setCache(key, p)
		return p
	}

	fallback := fallbackByRarity(rarity, itemType)
	if fallback > 0 {
		r.setCache(key, fallback)
	}
	return fallback
}

func (r *priceResolver) setCache(key string, price int64) {
	r.mu.Lock()
	r.cache[key] = cachedPrice{
		priceCents: price,
		expiresAt:  time.Now().Add(10 * time.Minute),
	}
	r.mu.Unlock()
}

func (r *priceResolver) fetchSteamPrice(ctx context.Context, name string) int64 {
	marketName := strings.TrimSpace(name)
	if marketName == "" {
		return 0
	}

	u := "https://steamcommunity.com/market/priceoverview/?appid=730&currency=1&market_hash_name=" + url.QueryEscape(marketName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0
	}

	var payload struct {
		Success     bool   `json:"success"`
		LowestPrice string `json:"lowest_price"`
		MedianPrice string `json:"median_price"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return 0
	}
	if !payload.Success {
		return 0
	}
	price := parseCurrencyToCents(payload.LowestPrice)
	if price <= 0 {
		price = parseCurrencyToCents(payload.MedianPrice)
	}
	return price
}

func parseCurrencyToCents(v string) int64 {
	re := regexp.MustCompile(`[^0-9.,]`)
	clean := re.ReplaceAllString(strings.TrimSpace(v), "")
	if clean == "" {
		return 0
	}
	clean = strings.ReplaceAll(clean, ",", "")
	if strings.Count(clean, ".") > 1 {
		return 0
	}
	f, err := strconv.ParseFloat(clean, 64)
	if err != nil || f <= 0 {
		return 0
	}
	return int64(f*100 + 0.5)
}

func staticKnownPrice(name string) int64 {
	key := strings.ToLower(strings.TrimSpace(name))
	switch key {
	case "revolution case":
		return 55
	case "kilowatt case":
		return 95
	case "knife fever case":
		return 1250
	case "dreams & nightmares case":
		return 140
	case "awp | wildfire":
		return 5200
	case "m4a1-s | cyrex":
		return 2100
	case "ak-47 | slate":
		return 1200
	case "karambit | doppler":
		return 150000
	case "m9 bayonet | fade":
		return 130000
	case "butterfly knife | slaughter":
		return 175000
	case "ump-45 | briefing":
		return 180
	case "mp9 | storm":
		return 80
	case "p250 | sand dune":
		return 25
	default:
		return 0
	}
}

func fallbackByRarity(rarity, itemType string) int64 {
	r := strings.ToLower(strings.TrimSpace(rarity))
	switch r {
	case "consumer":
		return 25
	case "industrial":
		return 75
	case "mil-spec", "milspec":
		return 220
	case "restricted":
		return 850
	case "classified":
		return 2200
	case "covert", "extraordinary":
		return 6200
	}
	if strings.ToLower(strings.TrimSpace(itemType)) == "case" {
		return 120
	}
	return 150
}

func maxPrice(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func extractPriceCents(metadata map[string]interface{}) int64 {
	if metadata == nil {
		return 0
	}
	v, ok := metadata["price_cents"]
	if !ok {
		return 0
	}
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	case float64:
		return int64(x)
	case json.Number:
		n, _ := x.Int64()
		return n
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(x), 10, 64)
		return n
	default:
		return 0
	}
}
