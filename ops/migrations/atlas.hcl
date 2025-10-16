env "lint" {
  dev = "postgres://postgres:postgres@localhost:15432/qz?sslmode=disable"

  dir = "file://ops/migrations/sql"

  lint {
    latest = 1
    destructive {
      error = true
    }
  }
}
