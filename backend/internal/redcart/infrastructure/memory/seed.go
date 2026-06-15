package memory

import (
	"fmt"
	"time"

	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	"golang.org/x/crypto/bcrypt"
)

func (r *Repository) seed() {
	now := time.Now().UTC()

	consumer, _ := r.CreateUser(domain.User{
		Nickname:     "Alice",
		Phone:        "13800000001",
		PasswordHash: seededPasswordHash("consumer-demo"),
		Role:         domain.RoleConsumer,
		CreatedAt:    now,
		UpdatedAt:    now,
	})

	merchantUser, _ := r.CreateUser(domain.User{
		Nickname:     "Merchant Zoe",
		Phone:        "13800000002",
		PasswordHash: seededPasswordHash("merchant-demo"),
		Role:         domain.RoleMerchant,
		CreatedAt:    now,
		UpdatedAt:    now,
	})

	merchant, _ := r.CreateMerchant(domain.Merchant{
		UserID:      merchantUser.ID,
		Name:        "RedCart Beauty Lab",
		Description: "Content-driven beauty merchant demo account",
		Status:      "active",
		CreatedAt:   now,
		UpdatedAt:   now,
	})

	productOne, _ := r.SaveProduct(domain.Product{
		MerchantID:    merchant.ID,
		Title:         "Velvet Lip Mud Set",
		Description:   "Matte lip set for commute, campus, and quick content shoots.",
		CoverURL:      "https://images.example.com/lip-mud.jpg",
		CategoryID:    101,
		Status:        domain.ProductStatusOnline,
		SellingPoints: []string{"Soft matte finish", "Pocket-size touch-up", "Daily shade bundle"},
		CreatedAt:     now,
		UpdatedAt:     now,
	})

	productTwo, _ := r.SaveProduct(domain.Product{
		MerchantID:    merchant.ID,
		Title:         "Travel Makeup Organizer",
		Description:   "Portable makeup storage for dorm, travel, and desk organization.",
		CoverURL:      "https://images.example.com/makeup-organizer.jpg",
		CategoryID:    102,
		Status:        domain.ProductStatusOnline,
		SellingPoints: []string{"Compartment layout", "Portable and lightweight", "Easy to clean"},
		CreatedAt:     now,
		UpdatedAt:     now,
	})

	productThree, _ := r.SaveProduct(domain.Product{
		MerchantID:    merchant.ID,
		Title:         "LED Desk Lamp",
		Description:   "Dimmable LED desk lamp with warm/cold light modes for small desk setups.",
		CoverURL:      "https://images.example.com/desk-lamp.jpg",
		CategoryID:    103,
		Status:        domain.ProductStatusOnline,
		SellingPoints: []string{"3 color temperatures", "USB powered", "Space saving"},
		CreatedAt:     now,
		UpdatedAt:     now,
	})

	productFour, _ := r.SaveProduct(domain.Product{
		MerchantID:    merchant.ID,
		Title:         "Desk Storage Box Set",
		Description:   "Multi-size storage boxes for stationery, cables, and daily desk items.",
		CoverURL:      "https://images.example.com/storage-box.jpg",
		CategoryID:    104,
		Status:        domain.ProductStatusOnline,
		SellingPoints: []string{"Stackable", "See-through lid", "Cable hole design"},
		CreatedAt:     now,
		UpdatedAt:     now,
	})

	productFive, _ := r.SaveProduct(domain.Product{
		MerchantID:    merchant.ID,
		Title:         "USB Power Strip",
		Description:   "Compact power strip with USB-C and USB-A ports for dorm desk electronics.",
		CoverURL:      "https://images.example.com/power-strip.jpg",
		CategoryID:    105,
		Status:        domain.ProductStatusOnline,
		SellingPoints: []string{"3 AC + 3 USB", "Overload protection", "1.8m cord"},
		CreatedAt:     now,
		UpdatedAt:     now,
	})

	skuOne, _ := r.SaveSKU(domain.SKU{
		ProductID:   productOne.ID,
		SKUName:     "Cherry Set",
		SKUAttrs:    map[string]string{"shade": "cherry", "pack": "3pcs"},
		PriceCent:   12900,
		Stock:       30,
		LockedStock: 0,
		Status:      domain.SKUStatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	})

	skuTwo, _ := r.SaveSKU(domain.SKU{
		ProductID:   productOne.ID,
		SKUName:     "Rose Set",
		SKUAttrs:    map[string]string{"shade": "rose", "pack": "3pcs"},
		PriceCent:   13900,
		Stock:       18,
		LockedStock: 0,
		Status:      domain.SKUStatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	})

	skuThree, _ := r.SaveSKU(domain.SKU{
		ProductID:   productTwo.ID,
		SKUName:     "Cream White",
		SKUAttrs:    map[string]string{"color": "cream", "size": "standard"},
		PriceCent:   8900,
		Stock:       40,
		LockedStock: 0,
		Status:      domain.SKUStatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	})

	skuFour, _ := r.SaveSKU(domain.SKU{
		ProductID:   productThree.ID,
		SKUName:     "White",
		SKUAttrs:    map[string]string{"color": "white", "size": "standard"},
		PriceCent:   7900,
		Stock:       50,
		LockedStock: 0,
		Status:      domain.SKUStatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	})

	skuFive, _ := r.SaveSKU(domain.SKU{
		ProductID:   productFour.ID,
		SKUName:     "3-Piece Set",
		SKUAttrs:    map[string]string{"color": "clear", "size": "3pcs"},
		PriceCent:   4900,
		Stock:       60,
		LockedStock: 0,
		Status:      domain.SKUStatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	})

	skuSix, _ := r.SaveSKU(domain.SKU{
		ProductID:   productFive.ID,
		SKUName:     "Standard",
		SKUAttrs:    map[string]string{"color": "white", "size": "standard"},
		PriceCent:   5900,
		Stock:       45,
		LockedStock: 0,
		Status:      domain.SKUStatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	})

	_, _ = r.SaveCartItem(domain.CartItem{
		UserID:    consumer.ID,
		ProductID: productOne.ID,
		SKUID:     skuOne.ID,
		Quantity:  1,
		Selected:  true,
		CreatedAt: now,
		UpdatedAt: now,
	})

	_ = skuTwo
	_ = skuFour
	_ = skuFive
	_ = skuSix

	noteOne := domain.Note{
		ID:         r.nextID(&r.nextNoteID),
		AuthorID:   merchantUser.ID,
		Title:      "通勤妆 5 分钟出门组合",
		Content:    "这套唇泥和整理盒是我最近拍通勤内容最常带的组合，颜色稳、补妆快、包里不乱。",
		CoverURL:   "https://images.example.com/note-commute.jpg",
		Status:     "published",
		ViewCount:  1280,
		LikeCount:  218,
		ProductIDs: []int64{productOne.ID, productTwo.ID},
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	r.notes[noteOne.ID] = cloneNote(noteOne)

	noteTwo := domain.Note{
		ID:         r.nextID(&r.nextNoteID),
		AuthorID:   merchantUser.ID,
		Title:      "宿舍桌面整理前后对比",
		Content:    "桌面一乱，化妆和出门效率都会掉。这个整理盒适合小桌面。",
		CoverURL:   "https://images.example.com/note-dorm.jpg",
		Status:     "published",
		ViewCount:  920,
		LikeCount:  141,
		ProductIDs: []int64{productTwo.ID},
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	r.notes[noteTwo.ID] = cloneNote(noteTwo)

	events := []domain.BehaviorEvent{
		{UserID: consumer.ID, EventType: domain.BehaviorNoteView, NoteID: noteOne.ID, MerchantID: merchant.ID, CreatedAt: now},
		{UserID: consumer.ID, EventType: domain.BehaviorNoteView, NoteID: noteTwo.ID, MerchantID: merchant.ID, CreatedAt: now},
		{UserID: consumer.ID, EventType: domain.BehaviorProductClick, ProductID: productOne.ID, MerchantID: merchant.ID, CreatedAt: now},
		{UserID: consumer.ID, EventType: domain.BehaviorProductClick, ProductID: productTwo.ID, MerchantID: merchant.ID, CreatedAt: now},
		{UserID: consumer.ID, EventType: domain.BehaviorAddToCart, ProductID: productOne.ID, SKUID: skuOne.ID, MerchantID: merchant.ID, CreatedAt: now},
		{UserID: consumer.ID, EventType: domain.BehaviorAddToCart, ProductID: productTwo.ID, SKUID: skuThree.ID, MerchantID: merchant.ID, CreatedAt: now},
	}
	for _, event := range events {
		_, _ = r.AppendBehaviorEvent(event)
	}

	r.seedOrderCreated(now.Add(-4*time.Hour), consumer.ID, merchant.ID, productOne, skuTwo, 1)
	r.seedOrderShipped(now.Add(-48*time.Hour), consumer.ID, merchant.ID, productTwo, skuThree, 1)
	r.seedOrderRefunding(now.Add(-24*time.Hour), consumer.ID, merchant.ID, productOne, skuOne, 1)
}

func (r *Repository) nextID(counter *int64) int64 {
	*counter = *counter + 1
	return *counter
}

func seededPasswordHash(password string) string {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		panic(fmt.Sprintf("seed password hash: %v", err))
	}
	return string(hash)
}
