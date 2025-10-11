package auth

import (
	"codesfer/internal/backend"
	"fmt"
	"log"
	"syscall"

	"golang.org/x/term"
)

func Register() {
	session := backend.ReadSessionID()
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
		fmt.Print("Username: ")
		fmt.Scanln(&username)
		usernameOk, err = backend.UsernameAvailable(username)
		if err != nil {
			log.Fatal(err)
		}
		if !usernameOk {
			fmt.Println("Username is not available. Please try again.")
		}
	}

	err = backend.Register(email, password, username)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Registration successful. Please login to continue.")
}
