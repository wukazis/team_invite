package main

import (
  "context"
  "fmt"

  "team-invite/internal/config"
  "team-invite/internal/database"
)

func main() {
  cfg, err := config.Load()
  if err != nil {
    panic(err)
  }
  store, err := database.New(context.Background(), cfg.PostgresURL)
  if err != nil {
    panic(err)
  }
  defer store.Close()
  codes, total, err := store.ListInviteCodes(context.Background(), 20, 0)
  fmt.Println("err=", err)
  fmt.Println("total=", total)
  fmt.Println("codes len=", len(codes))
}
