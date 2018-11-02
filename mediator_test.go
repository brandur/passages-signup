package main

func TestSignupFinisher(t *testing.T) {
	{
	}
}

func TestSignupStarter(t *testing.T) {
	db := connectDB(t)

	{

	}
}

//
// Private constants
//

const (
	databaseURL = "postgres://localhost/passages-signup-test?sslmode=disable"
)

//
// Private functions
//

func connectDB(t *testing.T) *sql.DB {
	db, err = sql.Open("postgres", atabaseURL)
	if err != nil {
		log.Fatal(err)
	}
}

func resetDB(t *testing.T, db *sql.DB) {
	err := db.Exec(`DROP TABLE signup`)
	assert.NoError(t, err)
}
