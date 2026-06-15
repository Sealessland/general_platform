package memory

import (
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
)

func cloneUser(user domain.User) domain.User {
	return user
}

func cloneMerchant(merchant domain.Merchant) domain.Merchant {
	return merchant
}

func cloneNote(note domain.Note) domain.Note {
	note.ProductIDs = domain.CloneInt64Slice(note.ProductIDs)
	return note
}

func cloneProduct(product domain.Product) domain.Product {
	product.SellingPoints = domain.CloneStringSlice(product.SellingPoints)
	return product
}

func cloneSKU(sku domain.SKU) domain.SKU {
	sku.SKUAttrs = domain.CloneMap(sku.SKUAttrs)
	return sku
}

func cloneCartItem(item domain.CartItem) domain.CartItem {
	return item
}

func cloneOrder(order domain.Order) domain.Order {
	if len(order.Items) == 0 {
		return order
	}
	items := make([]domain.OrderItem, len(order.Items))
	copy(items, order.Items)
	order.Items = items
	return order
}

func cloneOrderEvent(event domain.OrderEvent) domain.OrderEvent {
	return event
}

func cloneInventoryLock(lock domain.InventoryLock) domain.InventoryLock {
	return lock
}

func cloneAITask(task domain.AIGenerationTask) domain.AIGenerationTask {
	if task.Input != nil {
		input := make(map[string]any, len(task.Input))
		for key, value := range task.Input {
			input[key] = value
		}
		task.Input = input
	}
	if task.Output != nil {
		output := make(map[string]any, len(task.Output))
		for key, value := range task.Output {
			output[key] = value
		}
		task.Output = output
	}
	return task
}
