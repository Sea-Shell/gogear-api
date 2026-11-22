package models

// GearTopCategory represents a gear top category.
type GearTopCategory struct {
	TopCategoryID   *int64 `json:"top_category_id" db:"topCategoryId"`
	TopCategoryName string `json:"top_category_name" db:"topCategoryName"`
	TopCategoryIcon string `json:"top_category_icon" db:"topCategoryIcon"`
	// TopCategoryIsContainer is the top category's container setting (BR-021).
	// Absent or null on create means false; absent or null on update means
	// unchanged.
	TopCategoryIsContainer bool `json:"top_category_is_container" db:"topCategoryIsContainer"`
}
