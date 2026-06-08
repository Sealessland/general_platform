package application

import "github.com/example/redcart-copilot/backend/internal/redcart/domain"

func (s *Service) toUserView(user domain.User) UserView {
	view := UserView{
		ID:       user.ID,
		Nickname: user.Nickname,
		Phone:    user.Phone,
		Role:     user.Role,
	}
	if merchant, ok := s.repo.GetMerchantByUserID(user.ID); ok {
		view.MerchantID = merchant.ID
		view.Merchant = &MerchantView{
			ID:          merchant.ID,
			Name:        merchant.Name,
			Description: merchant.Description,
			Status:      merchant.Status,
		}
	}
	return view
}

func (s *Service) toNoteSummary(note domain.Note) NoteSummary {
	return NoteSummary{
		ID:             note.ID,
		Title:          note.Title,
		Content:        note.Content,
		CoverURL:       note.CoverURL,
		ViewCount:      note.ViewCount,
		LikeCount:      note.LikeCount,
		LinkedProducts: s.productCards(note.ProductIDs),
	}
}

func (s *Service) toNoteDetail(note domain.Note) NoteDetail {
	return NoteDetail{
		ID:             note.ID,
		Title:          note.Title,
		Content:        note.Content,
		CoverURL:       note.CoverURL,
		ViewCount:      note.ViewCount,
		LikeCount:      note.LikeCount,
		LinkedProducts: s.productCards(note.ProductIDs),
	}
}

func (s *Service) productCards(ids []int64) []ProductCard {
	out := make([]ProductCard, 0, len(ids))
	for _, id := range ids {
		product, ok := s.repo.GetProduct(id)
		if !ok {
			continue
		}
		out = append(out, s.toProductCard(product))
	}
	return out
}

func (s *Service) toProductCard(product domain.Product) ProductCard {
	minPrice := int64(0)
	stock := 0
	for _, sku := range s.repo.ListSKUsByProduct(product.ID) {
		if minPrice == 0 || sku.PriceCent < minPrice {
			minPrice = sku.PriceCent
		}
		stock += sku.Stock - sku.LockedStock
	}
	return ProductCard{
		ID:            product.ID,
		Title:         product.Title,
		CoverURL:      product.CoverURL,
		Status:        product.Status,
		MinPriceCent:  minPrice,
		Stock:         stock,
		SellingPoints: domain.CloneStringSlice(product.SellingPoints),
	}
}

func (s *Service) toProductDetail(product domain.Product) ProductDetail {
	detail := ProductDetail{
		ID:            product.ID,
		MerchantID:    product.MerchantID,
		Title:         product.Title,
		Description:   product.Description,
		CoverURL:      product.CoverURL,
		CategoryID:    product.CategoryID,
		Status:        product.Status,
		SellingPoints: domain.CloneStringSlice(product.SellingPoints),
		SKUs:          make([]SKUView, 0),
	}
	for _, sku := range s.repo.ListSKUsByProduct(product.ID) {
		detail.SKUs = append(detail.SKUs, s.toSKUView(sku))
	}
	return detail
}

func (s *Service) toSKUView(sku domain.SKU) SKUView {
	return SKUView{
		ID:          sku.ID,
		ProductID:   sku.ProductID,
		SKUName:     sku.SKUName,
		SKUAttrs:    domain.CloneMap(sku.SKUAttrs),
		PriceCent:   sku.PriceCent,
		Stock:       sku.Stock,
		LockedStock: sku.LockedStock,
		Status:      sku.Status,
	}
}
