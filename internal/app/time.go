package app

import "time"

// nowDay is the current local day in the storage layout.
func nowDay() string { return time.Now().Format("2006-01-02") }
