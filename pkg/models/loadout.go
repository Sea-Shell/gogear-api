package models

// ClosedPlace names a closed place of a loadout for the loadout summary. Name
// is the bag's gear name, "Worn" or "Last minute".
type ClosedPlace struct {
	PlaceID   int64  `json:"place_id"`
	PlaceKind string `json:"place_kind"`
	Name      string `json:"name"`
}

// Loadout represents a user gear grouping. TotalWeight is the on-your-back
// weight in grams, an exact integer that can exceed 64 bits (see WeightTotal);
// the counts and closed places summarize the loadout without
// reading its items (BR-029 AC-4). ItemCount counts item rows, not quantities.
type Loadout struct {
	LoadoutID          *int64 `json:"loadout_id" db:"loadoutId"`
	UserID             int64  `json:"user_id" db:"userId"`
	LoadoutName        string `json:"loadout_name" db:"loadoutName"`
	LoadoutDescription string `json:"loadout_description" db:"loadoutDescription"`
	LoadoutIsPublic    bool   `json:"loadout_is_public" db:"loadoutIsPublic"`
	LoadoutSlug        string `json:"loadout_slug" db:"loadoutSlug"`
	// TotalWeight is the on-your-back weight in grams: an exact JSON integer
	// that can exceed 2^63-1, never quoted or fractional. JavaScript's
	// JSON.parse rounds an integer above 2^53.
	TotalWeight WeightTotal `json:"total_weight" swaggertype:"integer"`
	ItemCount   int64       `json:"item_count"`
	TickedCount int64       `json:"ticked_count"`
	BagCount    int64       `json:"bag_count"`
	// ClosedPlaces is never nil: it encodes as [] when no place is closed.
	ClosedPlaces []ClosedPlace `json:"closed_places"`
	// Stage is "planning", "packing" or "done" (BR-028). DoneAt is when the
	// loadout was last set to done, and null unless Stage is "done". StartDate
	// and EndDate are calendar dates (YYYY-MM-DD) and WeightGoal is the target
	// on-your-back weight in grams; each is null when not set.
	Stage      string  `json:"stage" enums:"planning,packing,done"`
	DoneAt     *string `json:"done_at"`
	StartDate  *string `json:"start_date"`
	EndDate    *string `json:"end_date"`
	WeightGoal *int64  `json:"weight_goal"`
	CreatedAt  string  `json:"created_at" db:"createdAt"`
	UpdatedAt  string  `json:"updated_at" db:"updatedAt"`
}

// LoadoutFull is the whole board of one loadout, read from one transaction
// (ARS-001 section 3). Places are ordered Worn, Last minute, then bags by
// position; items by loadout_item_id.
type LoadoutFull struct {
	Loadout Loadout        `json:"loadout"`
	Places  []LoadoutPlace `json:"places"`
	Items   []LoadoutItem  `json:"items"`
}

// LoadoutNoID is used for creating new loadouts. A new loadout is always in
// stage planning, so the body has no stage (one named "planning" is accepted,
// any other is 400). The optional dates and goal follow BR-028 AC-4.
type LoadoutNoID struct {
	UserID             int64   `json:"user_id" db:"userId"`
	LoadoutName        string  `json:"loadout_name" db:"loadoutName"`
	LoadoutDescription string  `json:"loadout_description" db:"loadoutDescription"`
	LoadoutIsPublic    bool    `json:"loadout_is_public" db:"loadoutIsPublic"`
	LoadoutSlug        string  `json:"loadout_slug" db:"loadoutSlug"`
	StartDate          *string `json:"start_date,omitempty" format:"date"`
	EndDate            *string `json:"end_date,omitempty" format:"date"`
	WeightGoal         *int64  `json:"weight_goal,omitempty" minimum:"1"`
}

// LoadoutUpdate carries updatable fields with the ID for WHERE clause. Absent
// fields keep their value. start_date, end_date and weight_goal are clearable:
// null removes the stored value. stage is one of planning, packing and done
// and can change from any value to any other (BR-028 AC-2).
type LoadoutUpdate struct {
	LoadoutID          int64   `json:"loadout_id" db:"loadoutId"`
	LoadoutName        string  `json:"loadout_name" db:"loadoutName"`
	LoadoutDescription string  `json:"loadout_description" db:"loadoutDescription"`
	LoadoutIsPublic    bool    `json:"loadout_is_public" db:"loadoutIsPublic"`
	LoadoutSlug        string  `json:"loadout_slug" db:"loadoutSlug"`
	Stage              *string `json:"stage,omitempty" enums:"planning,packing,done"`
	StartDate          *string `json:"start_date,omitempty" format:"date"`
	EndDate            *string `json:"end_date,omitempty" format:"date"`
	WeightGoal         *int64  `json:"weight_goal,omitempty" minimum:"1"`
}

// LoadoutPublic is the public-facing response (no userId).
type LoadoutPublic struct {
	LoadoutID          int64  `json:"loadout_id" db:"loadoutId"`
	LoadoutName        string `json:"loadout_name" db:"loadoutName"`
	LoadoutDescription string `json:"loadout_description" db:"loadoutDescription"`
	LoadoutSlug        string `json:"loadout_slug" db:"loadoutSlug"`
	// TotalWeight is the on-your-back weight in grams: an exact JSON integer
	// that can exceed 2^63-1, never quoted or fractional. JavaScript's
	// JSON.parse rounds an integer above 2^53.
	TotalWeight WeightTotal `json:"total_weight" swaggertype:"integer"`
	CreatedAt   string      `json:"created_at" db:"createdAt"`
	UpdatedAt   string      `json:"updated_at" db:"updatedAt"`
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
	// The registration's own weight and count, read live (BR-026, BR-036). All
	// three are null for an item with no registration (legacy or deleted): it
	// reports only unit_weight and weight_source, as they were or as the
	// catalog has them.
	MeasuredWeight       *int64  `json:"measured_weight"`
	MeasuredWeightSource *string `json:"measured_weight_source"`
	ClosetCount          *int64  `json:"closet_count"`
	// Consumable is this item's role in this loadout (food, water or fuel,
	// BR-027). It belongs to the item, not to its registration.
	Consumable bool `json:"consumable" db:"consumable"`
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
	Consumable             *bool  `json:"consumable,omitempty"`
}

// LoadoutItemUpdate carries updatable fields with the ID for WHERE clause.
// place_id is clearable: absent keeps the place, null removes it. consumable
// is true or false; absent or null keeps it.
type LoadoutItemUpdate struct {
	LoadoutItemID int64  `json:"loadout_item_id" db:"loadoutItemId"`
	Quantity      int64  `json:"quantity" db:"quantity"`
	Notes         string `json:"notes" db:"notes"`
	PlaceID       *int64 `json:"place_id,omitempty"`
	Consumable    *bool  `json:"consumable,omitempty"`
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
