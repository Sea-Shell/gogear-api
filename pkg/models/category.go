package models

type GearCategory struct {
    CategoryId              *int64  `json:"category_id" db:"categoryId"`
    CategpryTopCategoryId   int64   `json:"category_top_category_id" db:"categoryTopCategoryId"`
    CategoryName            string  `json:"category_name" db:"categoryName"`
}

type GearCategoryListItem struct {
    CategoryId              *int64  `json:"category_id" db:"categoryId"`
    CategpryTopCategoryId   int64   `json:"category_top_category_id" db:"categoryTopCategoryId"`
    CategoryName            string  `json:"category_name" db:"categoryName"`
    TopCategoryIdId         int64   `json:"top_category_id" db:"topCategoryId"`
    TopCategoryName         string  `json:"top_category_name" db:"topCategoryName"`
}