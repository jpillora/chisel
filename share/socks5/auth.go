package socks5

import (
	"fmt"
	"io"
)

/*********************************
    Clients Negotiation:

    +----+----------+----------+
    |VER | NMETHODS | METHODS  |
    +----+----------+----------+
    | 1  |    1     | 1 to 255 |
    +----+----------+----------+
**********************************/

// AuthMethods
const (
	// AuthMethodNoAuth X'00' NO AUTHENTICATION REQUIRED
	AuthMethodNoAuth = uint8(0)

	// X'01' GSSAPI

	// AuthMethodUserPass X'02' USERNAME/PASSWORD
	AuthMethodUserPass = uint8(2)

	// X'03' to X'7F' IANA ASSIGNED

	// X'80' to X'FE' RESERVED FOR PRIVATE METHODS

	// AuthMethodNoAcceptable X'FF' NO ACCEPTABLE METHODS
	AuthMethodNoAcceptable = uint8(255)
)

/************************************************
    rfc1929 client user/pass negotiation req
    +----+------+----------+------+----------+
    |VER | ULEN |  UNAME   | PLEN |  PASSWD  |
    +----+------+----------+------+----------+
    | 1  |  1   | 1 to 255 |  1   | 1 to 255 |
    +----+------+----------+------+----------+
************************************************/
/************************************************
    rfc1929 server user/pass negotiation resp
                +----+--------+
                |VER | STATUS |
                +----+--------+
                | 1  |   1    |
                +----+--------+
************************************************/

const (
	// AuthUserPassVersion the VER field contains the current version
	// of the subnegotiation, which is X'01'
	AuthUserPassVersion = uint8(1)
	// AuthUserPassStatusSuccess a STATUS field of X'00' indicates success
	AuthUserPassStatusSuccess = uint8(0)
	// AuthUserPassStatusFailure if the server returns a `failure'
	// (STATUS value other than X'00') status, it MUST close the connection.
	AuthUserPassStatusFailure = uint8(1)
)

var (
	// ErrUserAuthFailed failed to authenticate
	ErrUserAuthFailed = fmt.Errorf("user authentication failed")
	// ErrNoSupportedAuth authenticate method not supported
	ErrNoSupportedAuth = fmt.Errorf("not supported authentication mechanism")
)

// AuthContext A Request encapsulates authentication state provided
// during negotiation
type AuthContext struct {
	// Provided auth method
	Method uint8
	// Payload provided during negotiation.
	// Keys depend on the used auth method.
	// For UserPassAuth contains Username
	Payload map[string]string
}

// Authenticator auth
type Authenticator interface {
	Authenticate(reader io.Reader, writer io.Writer) (*AuthContext, error)
	GetCode() uint8
}

// NoAuthAuthenticator is used to handle the "No Authentication" mode
type NoAuthAuthenticator struct{}

// GetCode implementation of Authenticator
func (a NoAuthAuthenticator) GetCode() uint8 {
	return AuthMethodNoAuth
}

// Authenticate implementation of Authenticator
func (a NoAuthAuthenticator) Authenticate(_ io.Reader, writer io.Writer) (*AuthContext, error) {
	_, err := writer.Write([]byte{socks5Version, AuthMethodNoAuth})
	return &AuthContext{AuthMethodNoAuth, nil}, err
}

// UserPassAuthenticator is used to handle username/password based
// authentication
type UserPassAuthenticator struct {
	Credentials CredentialStore
}

// GetCode implementation of Authenticator
func (a UserPassAuthenticator) GetCode() uint8 {
	return AuthMethodUserPass
}

// Authenticate implementation of Authenticator
func (a UserPassAuthenticator) Authenticate(reader io.Reader, writer io.Writer) (*AuthContext, error) {
	// Tell the client to use user/pass auth
	if _, err := writer.Write([]byte{socks5Version, AuthMethodUserPass}); err != nil {
		return nil, err
	}

	// Get the version and username length
	header := []byte{0, 0}
	if _, err := io.ReadAtLeast(reader, header, 2); err != nil {
		return nil, err
	}

	// Ensure we are compatible
	if header[0] != AuthUserPassVersion {
		return nil, fmt.Errorf("unsupported auth version: %v", header[0])
	}

	// Get the user name
	userLen := int(header[1])
	user := make([]byte, userLen)
	if _, err := io.ReadAtLeast(reader, user, userLen); err != nil {
		return nil, err
	}

	// Get the password length
	if _, err := reader.Read(header[:1]); err != nil {
		return nil, err
	}

	// Get the password
	passLen := int(header[0])
	pass := make([]byte, passLen)
	if _, err := io.ReadAtLeast(reader, pass, passLen); err != nil {
		return nil, err
	}

	// Verify the password
	if a.Credentials.Valid(string(user), string(pass)) {
		if _, err := writer.Write([]byte{AuthUserPassVersion, AuthUserPassStatusSuccess}); err != nil {
			return nil, err
		}
	} else {
		if _, err := writer.Write([]byte{AuthUserPassVersion, AuthUserPassStatusFailure}); err != nil {
			return nil, err
		}
		return nil, ErrUserAuthFailed
	}

	// Done
	return &AuthContext{AuthMethodUserPass, map[string]string{"Username": string(user)}}, nil
}

// authenticate is used to handle connection authentication
func (s *Server) authenticate(conn io.Writer, bufConn io.Reader) (*AuthContext, error) {
	// Get the methods
	methods, err := readMethods(bufConn)
	if err != nil {
		return nil, fmt.Errorf("failed to get auth methods: %v", err)
	}

	// Select a usable method
	for _, method := range methods {
		cator, found := s.authMethods[method]
		if found {
			return cator.Authenticate(bufConn, conn)
		}
	}

	// No usable method found
	return nil, noAcceptableAuth(conn)
}

// noAcceptableAuth is used to handle when we have no eligible
// authentication mechanism
func noAcceptableAuth(conn io.Writer) error {
	_, _ = conn.Write([]byte{socks5Version, AuthMethodNoAcceptable})
	return ErrNoSupportedAuth
}

// readMethods is used to read the number of methods
// and proceeding auth methods
func readMethods(r io.Reader) ([]byte, error) {
	header := []byte{0}
	if _, err := r.Read(header); err != nil {
		return nil, err
	}

	numMethods := int(header[0])
	methods := make([]byte, numMethods)
	_, err := io.ReadAtLeast(r, methods, numMethods)
	return methods, err
}
