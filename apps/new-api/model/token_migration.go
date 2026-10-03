package model

func MigrateServiceNameColumn() error {
	if DB.Migrator().HasColumn(&Token{}, "service_name") {
		return nil
	}
	return DB.Migrator().AddColumn(&Token{}, "service_name")
}
