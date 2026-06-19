-- Baseline: create all legacy tables
-- Uses IF NOT EXISTS for idempotent replay on existing databases

CREATE TABLE IF NOT EXISTS users (
    `userId` INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT DEFAULT 0,
    `userUsername` TEXT NOT NULL,
    `userPassword` TEXT NOT NULL,
    `userName` TEXT,
    `userEmail` TEXT NOT NULL,
    `userIsAdmin` INTEGER NOT NULL DEFAULT 0,
    `userIsExternal` INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS gear_top_category (
    `topCategoryId` INTEGER PRIMARY KEY AUTOINCREMENT DEFAULT 0,
    `topCategoryName` TEXT NOT NULL,
    `topCategoryIcon` TEXT NOT NULL DEFAULT 'spark'
);

CREATE TABLE IF NOT EXISTS gear_category (
    `categoryId` INTEGER PRIMARY KEY AUTOINCREMENT DEFAULT 0,
    `categoryTopCategoryId` INTEGER NOT NULL,
    `categoryName` TEXT NOT NULL,
    FOREIGN KEY (categoryTopCategoryId) REFERENCES gear_top_category(topCategoryId)
);

CREATE TABLE IF NOT EXISTS manufacture (
    `manufactureId` INTEGER PRIMARY KEY AUTOINCREMENT DEFAULT 0,
    `manufactureName` TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS gear (
    `gearId` INTEGER PRIMARY KEY AUTOINCREMENT DEFAULT 0,
    `gearTopCategoryId` INTEGER NOT NULL,
    `gearCategoryId` INTEGER NOT NULL,
    `gearManufactureId` INTEGER NOT NULL,
    `gearIsContainer` INTEGER NOT NULL DEFAULT 0,
    `gearSizeDefinition` TEXT DEFAULT "",
    `gearName` TEXT NOT NULL,
    `gearWeight` INTEGER,
    `gearHeight` INTEGER,
    `gearLength` INTEGER,
    `gearWidth` INTEGER,
    `gearStatus` BOOLEAN,
    FOREIGN KEY (gearTopCategoryId) REFERENCES gear_top_category(topCategoryId),
    FOREIGN KEY (gearCategoryId) REFERENCES gear_category(categoryId),
    FOREIGN KEY (gearManufactureId) REFERENCES manufacture(manufactureId)
);

CREATE INDEX IF NOT EXISTS topCategory_index ON gear(gearTopCategoryId);
CREATE INDEX IF NOT EXISTS category_index ON gear(gearCategoryId);
CREATE INDEX IF NOT EXISTS manufacture_index ON gear(gearManufactureId);

CREATE TABLE IF NOT EXISTS user_gear_registrations (
    `userGearRegistrationId` INTEGER PRIMARY KEY AUTOINCREMENT DEFAULT 0,
    `gearId` INTEGER NOT NULL,
    `userId` INTEGER NOT NULL,
    `maxContainerWeight` INTEGER,
    FOREIGN KEY (gearId) REFERENCES gear(gearId),
    FOREIGN KEY (userId) REFERENCES users(userId)
);

CREATE TABLE IF NOT EXISTS user_container_registration (
    `containerRegistrationId` INTEGER PRIMARY KEY AUTOINCREMENT DEFAULT 0,
    `userContainerId` INTEGER NOT NULL,
    `userGearRegistrationId` INTEGER NOT NULL,
    FOREIGN KEY (userContainerId) REFERENCES user_gear_registrations(userGearRegistrationId) ON DELETE CASCADE,
    FOREIGN KEY (userGearRegistrationId) REFERENCES user_gear_registrations(userGearRegistrationId) ON DELETE CASCADE
);
