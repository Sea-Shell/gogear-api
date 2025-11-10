package models

type GearCategory struct {
	CategoryID            *int64 `json:"category_id" db:"categoryId"`
	CategoryTopCategoryID int64  `json:"category_top_category_id" db:"categoryTopCategoryId"`
	CategoryName          string `json:"category_name" db:"categoryName"`
}

type GearCategoryListItem struct {
	CategoryID            *int64 `json:"category_id" db:"categoryId"`
	CategoryTopCategoryID int64  `json:"category_top_category_id" db:"categoryTopCategoryId"`
	CategoryName          string `json:"category_name" db:"categoryName"`
	TopCategoryID         int64  `json:"top_category_id" db:"topCategoryId"`
	TopCategoryName       string `json:"top_category_name" db:"topCategoryName"`
	TopCategoryIcon       string `json:"top_category_icon" db:"topCategoryIcon"`
}
