package service

import (
    "strings"
    "testing"
    "time"

    "github.com/HasinduNimesh/ClapTac-tech-triathlon/pkg/authorization"
    "github.com/HasinduNimesh/ClapTac-tech-triathlon/services/delivery-service/internal/domain"
)

func TestLocationRequiresActiveAssignedTrip(t *testing.T) {
    now := time.Now().UTC()
    driver := &authorization.Profile{Roles: []string{authorization.RoleDriver}, VehicleID: "VEH001"}
    run := domain.Run{Status: domain.RunInProgress, VehicleID: "VEH001"}
    if err := validateLocation(driver, run, 6.9271, 79.8612, now, now); err != nil { t.Fatal(err) }
    run.Status = domain.RunCompleted
    if err := validateLocation(driver, run, 6.9271, 79.8612, now, now); err == nil || !strings.HasPrefix(err.Error(), "conflict") {
        t.Fatalf("completed trip accepted location: %v", err)
    }
    run.Status = domain.RunPrepared
    if err := validateLocation(driver, run, 6.9271, 79.8612, now, now); err == nil { t.Fatal("prepared trip accepted location") }
    run.Status = domain.RunInProgress
    driver.VehicleID = "VEH002"
    if err := validateLocation(driver, run, 6.9271, 79.8612, now, now); err == nil || !strings.HasPrefix(err.Error(), "forbidden") {
        t.Fatalf("other vehicle accepted location: %v", err)
    }
    driver.VehicleID = "VEH001"
    if err := validateLocation(driver, run, 91, 79.8612, now, now); err == nil { t.Fatal("invalid latitude accepted") }
    if err := validateLocation(driver, run, 6.9271, 79.8612, now.Add(-10*time.Minute), now); err == nil { t.Fatal("stale point accepted") }
}
