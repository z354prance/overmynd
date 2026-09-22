package integrations

import (
	"encoding/json"
	"fmt"
	"strings"
)

type UsernamePasswordCredential struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func EncodeUsernamePassword(
	username string,
	password string,
) (string, error) {
	credential := UsernamePasswordCredential{
		Username: strings.TrimSpace(username),
		Password: password,
	}

	data, err := json.Marshal(credential)
	if err != nil {
		return "", fmt.Errorf("encode credentials: %w", err)
	}

	return string(data), nil
}

func DecodeUsernamePassword(
	value string,
) (UsernamePasswordCredential, error) {
	var credential UsernamePasswordCredential

	if strings.TrimSpace(value) == "" {
		return credential, fmt.Errorf("credentials are required")
	}

	if err := json.Unmarshal(
		[]byte(value),
		&credential,
	); err != nil {
		return credential, fmt.Errorf(
			"decode credentials: %w",
			err,
		)
	}

	credential.Username = strings.TrimSpace(
		credential.Username,
	)

	if credential.Username == "" {
		return credential, fmt.Errorf("username is required")
	}

	return credential, nil
}
