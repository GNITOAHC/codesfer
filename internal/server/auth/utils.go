package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"
)

func generateUniqueID() string {
	// Get the current time in nanoseconds
	currentTime := time.Now().UnixNano()

	// Convert the current time to a byte array
	timeBytes := fmt.Appendf(nil, "%d", currentTime)

	// Create a new SHA256 hash
	hash := sha256.New()
	hash.Write(timeBytes)

	// Get the hashed bytes and convert to a hex string
	hashedID := hash.Sum(nil)
	return hex.EncodeToString(hashedID)
}

type LocationResponse struct {
	City    string `json:"city"`
	Region  string `json:"region"`
	Country string `json:"country"`
	Bogon   bool   `json:"bogon"`
}

// getLocation returns the location as a string based on the IP address
func ip2Location(ip string) (string, error) {
	// Example API endpoint (you can use any geolocation API)
	apiURL := fmt.Sprintf("https://ipinfo.io/%s/json", ip)

	resp, err := http.Get(apiURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get location: status code %d", resp.StatusCode)
	}

	var locResp LocationResponse
	if err := json.NewDecoder(resp.Body).Decode(&locResp); err != nil {
		return "", err
	}

	if locResp.Bogon {
		return "Localhost", nil
	}

	if locResp.City == "" && locResp.Region == "" && locResp.Country == "" {
		return "Unknown", nil
	}

	location := fmt.Sprintf("%s, %s, %s", locResp.City, locResp.Region, locResp.Country)
	return location, nil
}

// verify will verify the user with it's email and password
func verify(email, password string) (bool, error) {
	user, err := getUser(email)
	if err != nil {
		return false, err
	}
	if user == nil {
		log.Printf("[verify] user not found: %s", email)
		return false, errors.New("user not found")
	}
	return checkPassword(password, user.Password), nil
}
