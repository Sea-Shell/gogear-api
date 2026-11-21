package models

// GearCategory is a sub-category as the API reports it. CategoryContainer is
// the explicit container setting ("container", "not_container" or "inherit")
// and CategoryIsContainer the effective setting, both read from the view
// category_container_setting (BR-021, ADR-001 D-2, D-3). CategoryIsContainer is
// read-only and ignored when sent.
type GearCategory struct {
	CategoryID            *int64 `json:"category_id" db:"gear_category.categoryId"`
	CategoryTopCategoryID int64  `json:"category_top_category_id" db:"gear_category.categoryTopCategoryId"`
	CategoryName          string `json:"category_name" db:"gear_category.categoryName"`
	CategoryContainer     string `json:"category_container" db:"CASE category_container_setting.explicitIsContainer WHEN 1 THEN 'container' WHEN 0 THEN 'not_container' ELSE 'inherit' END" enums:"container,not_container,inherit"`
	CategoryIsContainer   bool   `json:"category_is_container" db:"COALESCE(category_container_setting.effectiveIsContainer, 0)" readonly:"true"`
}

// GearCategoryInput is the request body for creating or updating a
// sub-category. CategoryContainer is optional: absent or null means "inherit"
// on create and "unchanged" on update. category_is_container is not accepted.
type GearCategoryInput struct {
	CategoryID            *int64  `json:"category_id,omitempty"`
	CategoryTopCategoryID int64   `json:"category_top_category_id"`
	CategoryName          string  `json:"category_name"`
	CategoryContainer     *string `json:"category_container,omitempty" enums:"container,not_container,inherit"`
}

type GearCategoryListItem struct {
	CategoryID             *int64 `json:"category_id" db:"gear_category.categoryId"`
	CategoryTopCategoryID  int64  `json:"category_top_category_id" db:"gear_category.categoryTopCategoryId"`
	CategoryName           string `json:"category_name" db:"gear_category.categoryName"`
	CategoryContainer      string `json:"category_container" db:"CASE category_container_setting.explicitIsContainer WHEN 1 THEN 'container' WHEN 0 THEN 'not_container' ELSE 'inherit' END" enums:"container,not_container,inherit"`
	CategoryIsContainer    bool   `json:"category_is_container" db:"COALESCE(category_container_setting.effectiveIsContainer, 0)" readonly:"true"`
	TopCategoryID          int64  `json:"top_category_id" db:"topCategoryId"`
	TopCategoryName        string `json:"top_category_name" db:"topCategoryName"`
	TopCategoryIcon        string `json:"top_category_icon" db:"topCategoryIcon"`
	TopCategoryIsContainer bool   `json:"top_category_is_container" db:"topCategoryIsContainer"`
}
