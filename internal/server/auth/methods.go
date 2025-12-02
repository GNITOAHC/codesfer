package auth

func UsernameFromSessionID(sessionID string) (string, error) {
	session, err := getSession(sessionID)
	if err != nil {
		return "", err
	}
	user, err := getUser(session.Email)
	if err != nil {
		return "", err
	}
	return user.Username, nil
}
