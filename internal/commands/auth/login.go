package auth

import (
	"codesfer/internal/backend"
	"fmt"
	"log"
	"syscall"

	"golang.org/x/term"
)

// func init() {
// 	backend.Init("http://localhost:3000")
// }

func Login() {
	session := backend.ReadSessionID()
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

	sessionID, err := backend.Login(email, password)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Login successful. Session ID: %s\n", sessionID)
	backend.WriteSessionID(sessionID)
}
