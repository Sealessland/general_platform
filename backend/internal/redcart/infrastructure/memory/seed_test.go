package memory

import (
	"testing"
)

func TestRepositorySeededDataAndCloneBoundaries(t *testing.T) {
	repo := NewRepository()

	notes := repo.ListNotes()
	if len(notes) == 0 {
		t.Fatal("expected seeded notes")
	}
	note, ok := repo.GetNote(notes[0].ID)
	if !ok || len(note.ProductIDs) == 0 {
		t.Fatalf("expected seeded note with products, got %+v", note)
	}
	note.ProductIDs[0] = 999999
	refetchedNote, _ := repo.GetNote(notes[0].ID)
	if refetchedNote.ProductIDs[0] == 999999 {
		t.Fatal("expected note product ids to be cloned")
	}

	product, ok := repo.GetProduct(1)
	if !ok || len(product.SellingPoints) == 0 {
		t.Fatalf("expected seeded product, got %+v", product)
	}
	product.SellingPoints[0] = "mutated"
	refetchedProduct, _ := repo.GetProduct(1)
	if refetchedProduct.SellingPoints[0] == "mutated" {
		t.Fatal("expected product selling points to be cloned")
	}

	sku, ok := repo.GetSKU(1)
	if !ok || len(sku.SKUAttrs) == 0 {
		t.Fatalf("expected seeded sku, got %+v", sku)
	}
	sku.SKUAttrs["shade"] = "mutated"
	refetchedSKU, _ := repo.GetSKU(1)
	if refetchedSKU.SKUAttrs["shade"] == "mutated" {
		t.Fatal("expected sku attrs to be cloned")
	}
}
