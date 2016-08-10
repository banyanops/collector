package collector

import (
	"fmt"
	"testing"
	"time"
)

func TestRateLimiter(t *testing.T) {
	fmt.Println("TestRateLimiter")

	RegistryLimiterWait()
	fmt.Println("No rate limiter test succeeded")

	fmt.Println("Add a couple of rate limiters")
	period, err := time.ParseDuration("5s")
	if err != nil {
		t.Fatal(err)
	}
	err = AddRegistryRateLimiter(10, period)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("Added limiter")

	period, err = time.ParseDuration("1h")
	if err != nil {
		t.Fatal(err)
	}
	err = AddRegistryRateLimiter(30, period)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("Added limiter")
	for i := 0; i < 20; i++ {
		RegistryLimiterWait()
		fmt.Printf("Request %d time %v\n", i, time.Now().Format("03:04:05"))
	}

	fmt.Println("Stopping all rate limiters")
	DelRegistryRateLimiters()

	return
}
