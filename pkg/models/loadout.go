package models

// Loadout represents a user gear grouping.
type Loadout struct {
	LoadoutID          *int64 `json:"loadout_id" db:"loadoutId"`
	UserID             int64  `json:"user_id" db:"userId"`
	LoadoutName        string `json:"loadout_name" db:"loadoutName"`
	LoadoutDescription string `json:"loadout_description" db:"loadoutDescription"`
	LoadoutIsPublic    bool   `json:"loadout_is_public" db:"loadoutIsPublic"`
	LoadoutSlug        string `json:"loadout_slug" db:"loadoutSlug"`
	TotalWeight        int64  `json:"total_weight" db:"totalWeight"`
	CreatedAt          string `json:"created_at" db:"createdAt"`
	UpdatedAt          string `json:"updated_at" db:"updatedAt"`
}

// LoadoutNoID is used for creating new loadouts.
type LoadoutNoID struct {
	UserID             int64  `json:"user_id" db:"userId"`
	LoadoutName        string `json:"loadout_name" db:"loadoutName"`
	LoadoutDescription string `json:"loadout_description" db:"loadoutDescription"`
	LoadoutIsPublic    bool   `json:"loadout_is_public" db:"loadoutIsPublic"`
	LoadoutSlug        string `json:"loadout_slug" db:"loadoutSlug"`
}

// LoadoutUpdate carries updatable fields with the ID for WHERE clause.
type LoadoutUpdate struct {
	LoadoutID          int64  `json:"loadout_id" db:"loadoutId"`
	LoadoutName        string `json:"loadout_name" db:"loadoutName"`
	LoadoutDescription string `json:"loadout_description" db:"loadoutDescription"`
	LoadoutIsPublic    bool   `json:"loadout_is_public" db:"loadoutIsPublic"`
	LoadoutSlug        string `json:"loadout_slug" db:"loadoutSlug"`
}

// LoadoutPublic is the public-facing response (no userId).
type LoadoutPublic struct {
	LoadoutID          int64  `json:"loadout_id" db:"loadoutId"`
	LoadoutName        string `json:"loadout_name" db:"loadoutName"`
	LoadoutDescription string `json:"loadout_description" db:"loadoutDescription"`
	LoadoutSlug        string `json:"loadout_slug" db:"loadoutSlug"`
	TotalWeight        int64  `json:"total_weight" db:"totalWeight"`
	CreatedAt          string `json:"created_at" db:"createdAt"`
	UpdatedAt          string `json:"updated_at" db:"updatedAt"`
}

// LoadoutItem represents a single gear item within a loadout.
type LoadoutItem struct {
	LoadoutItemID *int64 `json:"loadout_item_id" db:"loadoutItemId"`
	LoadoutID     int64  `json:"loadout_id" db:"loadoutId"`
	GearID        int64  `json:"gear_id" db:"gearId"`
	Quantity      int64  `json:"quantity" db:"quantity"`
	Notes         string `json:"notes" db:"notes"`
}

// LoadoutItemNoID is used for adding gear to a loadout.
type LoadoutItemNoID struct {
	LoadoutID int64  `json:"loadout_id" db:"loadoutId"`
	GearID    int64  `json:"gear_id" db:"gearId"`
	Quantity  int64  `json:"quantity" db:"quantity"`
	Notes     string `json:"notes" db:"notes"`
}

// LoadoutItemUpdate carries updatable fields with the ID for WHERE clause.
type LoadoutItemUpdate struct {
	LoadoutItemID int64  `json:"loadout_item_id" db:"loadoutItemId"`
	Quantity      int64  `json:"quantity" db:"quantity"`
	Notes         string `json:"notes" db:"notes"`
}
