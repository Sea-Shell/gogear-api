package models

// GearTopCategory represents a gear top category.
type GearTopCategory struct {
	TopCategoryID   *int64 `json:"top_category_id" db:"topCategoryId"`
	TopCategoryName string `json:"top_category_name" db:"topCategoryName"`
}
