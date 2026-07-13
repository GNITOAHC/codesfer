package cli

import (
	"fmt"
	"log"
	"syscall"
	"time"

	"github.com/gnitoahc/codesfer/internal/client"

	"golang.org/x/term"
)

// Account shows user's email, username, and all active sessions.
func Account() {
	account, err := client.AccountInfo(client.ReadSessionID())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Email: %s\n", account.Email)
	fmt.Printf("Username: %s\n", account.Username)
	for i, session := range account.Sessions {
		if session.Current {
			fmt.Printf("%s", "\033[36m") // cyan
		}
		fmt.Printf(
			"[%d] Session: Location: %s, Agent: %s, Last seen: %s, Created at: %s (name: %s)\n",
			i, session.Location, session.Agent, formatLastSeen(session.LastSeen), time.Unix(session.CreatedAt, 0).Format("2006-01-02"), session.Name,
		)
		if session.Current {
			fmt.Printf("%s", "\033[0m") // reset
		}
	}
}

// formatLastSeen formats the last seen timestamp based on its relation to the current date.
func formatLastSeen(lastSeen int64) string {
	t := time.Unix(lastSeen, 0)
	now := time.Now()
	if t.Year() == now.Year() && t.Month() == now.Month() && t.Day() == now.Day() {
		// Today: "today HH:MM"
		return fmt.Sprintf("today %s", t.Format("15:04"))
	} else if t.Year() == now.Year() {
		// This year: "Month Day" (e.g., "Jan 02")
		return t.Format("Jan 02")
	} else {
		// Not this year: "Year Month Day" (e.g., "2006 Jan 02")
		return t.Format("2006 Jan 02")
	}
}

// Login authenticates the user and stores the session locally.
func Login() {
	session := client.ReadSessionID()
	if session != "" {
		fmt.Println("You are already logged in. Logout first to sign in to different account.")
		return
	}

	var email string
	var password string

	fmt.Print("Email: ")
	fmt.Scanln(&email)
	fmt.Print("Password: ")
	bytePassword, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println() // move to next line after input
	password = string(bytePassword)

	sessionID, err := client.Login(email, password)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Login successful. Session ID: %s\n", sessionID)
	client.WriteSessionID(sessionID)
}

// Logout removes the remote and local sessions.
// If sessionIndex is -1, it logs out the current session.
// Otherwise, it logs out the session at the given index.
func Logout(sessionIndex int) {
	sessionID := client.ReadSessionID()
	if sessionID == "" {
		fmt.Println("You are not logged in.")
		return
	}

	if sessionIndex == -1 {
		// Logout current session
		err := client.Logout(sessionID)
		if err != nil {
			log.Println("Session not deleted or not found:", err)
		}

		if err := client.RemoveSessionID(); err != nil {
			log.Fatal(err)
		}
		fmt.Println("Logout successful.")
		return
	}

	// Logout a specific session by index
	account, err := client.AccountInfo(sessionID)
	if err != nil {
		log.Fatal(err)
	}

	if sessionIndex < 0 || sessionIndex >= len(account.Sessions) {
		fmt.Printf("Invalid session index. Valid range: 0-%d\n", len(account.Sessions)-1)
		return
	}

	targetSession := account.Sessions[sessionIndex]
	if targetSession.Current {
		// If trying to logout current session via index, just do regular logout
		err := client.Logout(sessionID)
		if err != nil {
			log.Println("Session not deleted or not found:", err)
		}

		if err := client.RemoveSessionID(); err != nil {
			log.Fatal(err)
		}
		fmt.Println("Logout successful.")
		return
	}

	err = client.LogoutSession(sessionID, targetSession.Name)
	if err != nil {
		log.Fatal("Failed to logout session:", err)
	}
	fmt.Printf("Session [%d] has been logged out.\n", sessionIndex)
}

// Register signs up a new account with email/password/username.
func Register() {
	session := client.ReadSessionID()
	if session != "" {
		fmt.Println("You are logged in. Logout first to register for an account.")
		return
	}

	var email string
	var password string
	var confirmPassword string
	var username string

	fmt.Print("Email: ")
	fmt.Scanln(&email)

	fmt.Print("Password: ")
	bytePass, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println() // move to next line after input
	password = string(bytePass)

	fmt.Print("Confirm your password again: ")
	confirmBytePass, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println() // move to next line after input
	confirmPassword = string(confirmBytePass)

	if password != confirmPassword {
		fmt.Println("Passwords do not match. Please try again.")
		return
	}

	usernameOk := false
	for !usernameOk {
		fmt.Printf("Username (%sOnly single word is allowed%s): ", "\033[36m", "\033[0m") // cyan
		fmt.Scanln(&username)

		fmt.Printf("Checking username: {%s} availability...\n", username)
		if username == "" {
			fmt.Println("Username cannot be empty. Please try again.")
			continue
		}

		usernameOk, err = client.UsernameAvailable(username)
		if err != nil {
			log.Fatal(err)
		}
		if !usernameOk {
			fmt.Println("Username is not available. Please try again.")
		}
	}

	if err := client.Register(email, password, username); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Registration successful. Please login to continue.")
}
