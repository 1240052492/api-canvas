package model

func MigrateServiceNameColumn() error {
	// Check if table exists first
	if !DB.Migrator().HasTable(&Token{}) {
		return nil
	}
	if DB.Migrator().HasColumn(&Token{}, "service_name") {
		return nil
	}
	return DB.Migrator().AddColumn(&Token{}, "service_name")
}
