package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadBudgetLimits(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := "currency: CNY\nrate: 7.2\nbudget:\n  daily: 25\n  monthly: 300\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	s, _, err := Load(Paths{Home: dir, Config: path})
	if err != nil {
		t.Fatal(err)
	}
	if s.BudgetDaily != 25 || s.BudgetMonthly != 300 {
		t.Fatalf("budget = %g/%g", s.BudgetDaily, s.BudgetMonthly)
	}
}

func TestBudgetUnsetByDefault(t *testing.T) {
	s, _, err := Load(Paths{Home: t.TempDir(), Config: filepath.Join(t.TempDir(), "missing.yaml")})
	if err != nil {
		t.Fatal(err)
	}
	if s.BudgetDaily != 0 || s.BudgetMonthly != 0 {
		t.Fatalf("unexpected defaults %g/%g", s.BudgetDaily, s.BudgetMonthly)
	}
}
