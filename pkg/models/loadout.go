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

// LoadoutItem represents a single item within a loadout (ARS-001 section 3,
// milestone A). A registration-backed item names the owner's registration; a
// detached item (registration_exists = false) is a legacy item that reads its
// gear live, or a snapshot kept after its registration was deleted.
type LoadoutItem struct {
	LoadoutItemID *int64 `json:"loadout_item_id" db:"loadoutItemId"`
	LoadoutID     int64  `json:"loadout_id" db:"loadoutId"`
	// GearID is read-only: the registration's gear, else the detached gear. It
	// is null only for a deleted registration whose gear was already missing.
	GearID                 *int64  `json:"gear_id" db:"gearId"`
	Quantity               int64   `json:"quantity" db:"quantity"`
	Notes                  string  `json:"notes" db:"notes"`
	UsergearRegistrationID *int64  `json:"usergear_registration_id" db:"registrationId"`
	RegistrationExists     bool    `json:"registration_exists"`
	DetachedReason         *string `json:"detached_reason" db:"detachedReason"`
	// Names are read live from the current gear, except for an item whose
	// registration was deleted: its gear_name is the snapshot and the other
	// names are null.
	GearName        string  `json:"gear_name"`
	ManufactureName *string `json:"manufacture_name"`
	CategoryName    *string `json:"category_name"`
	TopCategoryName *string `json:"top_category_name"`
	PlaceID         *int64  `json:"place_id" db:"placeId"`
	Ticked          bool    `json:"ticked" db:"ticked"`
	CatalogWeight   *int64  `json:"catalog_weight"`
	UnitWeight      *int64  `json:"unit_weight"`
	WeightSource    string  `json:"weight_source"`
}

// LoadoutItemNoID is used for adding an item to a loadout. Send
// usergear_registration_id; gear_id alone is the deprecated legacy path
// (ADR-004 D-2) and is refused together with usergear_registration_id.
type LoadoutItemNoID struct {
	LoadoutID              int64  `json:"loadout_id,omitempty" db:"loadoutId"`
	UsergearRegistrationID *int64 `json:"usergear_registration_id,omitempty"`
	GearID                 *int64 `json:"gear_id,omitempty" db:"gearId"`
	Quantity               int64  `json:"quantity" db:"quantity"`
	Notes                  string `json:"notes" db:"notes"`
	PlaceID                *int64 `json:"place_id,omitempty"`
}

// LoadoutItemUpdate carries updatable fields with the ID for WHERE clause.
// place_id is clearable: absent keeps the place, null removes it.
type LoadoutItemUpdate struct {
	LoadoutItemID int64  `json:"loadout_item_id" db:"loadoutItemId"`
	Quantity      int64  `json:"quantity" db:"quantity"`
	Notes         string `json:"notes" db:"notes"`
	PlaceID       *int64 `json:"place_id,omitempty"`
}

// LoadoutItemTick is the body of the tick request.
type LoadoutItemTick struct {
	Ticked bool `json:"ticked"`
}

// LoadoutItemTickResult is the response of the tick request.
type LoadoutItemTickResult struct {
	LoadoutItemID int64 `json:"loadout_item_id"`
	Ticked        bool  `json:"ticked"`
}

// LoadoutItemPublic is an item of a public loadout.
type LoadoutItemPublic struct {
	LoadoutItemID *int64 `json:"loadout_item_id"`
	LoadoutID     int64  `json:"loadout_id"`
	GearID        int64  `json:"gear_id"`
	Quantity      int64  `json:"quantity"`
	Notes         string `json:"notes"`
	GearName      string `json:"gear_name"`
}
