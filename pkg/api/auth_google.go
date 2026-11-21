package endpoints

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/Sea-Shell/gogear-api/pkg/models"
	"github.com/gin-gonic/gin"
	"google.golang.org/api/idtoken"
)

// GoogleIdentity is the validated content of a Google ID token that sign-in
// uses.
type GoogleIdentity struct {
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
}

// GoogleTokenValidator validates a raw Google ID token for an audience.
type GoogleTokenValidator func(ctx context.Context, rawToken, audience string) (*GoogleIdentity, error)

// googleValidatorContextKey lets a caller replace token validation (tests
// inject payloads without network access). Production never sets it.
const googleValidatorContextKey = "google_token_validator"

func googleValidatorFrom(c *gin.Context) GoogleTokenValidator {
	if v, ok := c.Get(googleValidatorContextKey); ok {
		if fn, ok := v.(GoogleTokenValidator); ok {
			return fn
		}
	}
	return validateGoogleIDToken
}

// validateGoogleIDToken verifies the token with Google's keys and extracts the
// claims sign-in needs.
func validateGoogleIDToken(ctx context.Context, rawToken, audience string) (*GoogleIdentity, error) {
	payload, err := idtoken.Validate(ctx, rawToken, audience)
	if err != nil {
		return nil, err
	}
	id := &GoogleIdentity{Subject: strings.TrimSpace(payload.Subject)}
	claims := payload.Claims
	if v, ok := claims["email"].(string); ok {
		id.Email = strings.TrimSpace(v)
	}
	switch v := claims["email_verified"].(type) {
	case bool:
		id.EmailVerified = v
	case string:
		id.EmailVerified = v == "true"
	}
	if v, ok := claims["name"].(string); ok {
		id.Name = strings.TrimSpace(v)
	}
	if id.Name == "" {
		given, _ := claims["given_name"].(string)
		family, _ := claims["family_name"].(string)
		id.Name = strings.TrimSpace(given + " " + family)
	}
	return id, nil
}

var (
	// errGoogleRefused: the sign-in must not link or create anything (403).
	errGoogleRefused = errors.New("google sign-in refused")
	// errGoogleEmailMissing: a new account cannot be created without an email.
	errGoogleEmailMissing = errors.New("google token missing email")
)

func refuse(reason string) error { return fmt.Errorf("%w: %s", errGoogleRefused, reason) }

// signInWithGoogle resolves the account for a validated Google identity
// (BR-003 to BR-005):
//
//  1. an account bound to the subject signs in; email is ignored;
//  2. otherwise a verified email may backfill the subject of exactly one
//     unbound account whose owner has never edited its email;
//  3. otherwise a verified email that no account uses creates a non-admin
//     account bound to the subject;
//  4. anything else is refused and changes nothing.
//
// It runs in one transaction (the database takes the write lock at begin), so
// racing sign-ins cannot bind two subjects to one account.
func signInWithGoogle(ctx context.Context, db *sql.DB, id *GoogleIdentity) (*models.User, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after commit

	user, err := resolveGoogleAccount(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return user, nil
}

func resolveGoogleAccount(ctx context.Context, tx *sql.Tx, id *GoogleIdentity) (*models.User, error) {
	// BR-003: the subject alone identifies a bound account.
	if user, err := queryGoogleUser(ctx, tx, `googleSubject = ?`, id.Subject); err == nil {
		return user, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	email := strings.TrimSpace(id.Email)
	if email == "" {
		return nil, errGoogleEmailMissing
	}
	if !id.EmailVerified {
		return nil, refuse("email not verified and subject not bound")
	}

	// An account that uses the email but cannot be backfilled blocks creation
	// (BR-005 AC-2), so count every account with the email first.
	rows, err := tx.QueryContext(ctx,
		`SELECT userId, googleSubject IS NOT NULL, userEmailEdited FROM users WHERE trim(userEmail) = ?`, email)
	if err != nil {
		return nil, err
	}
	var all, eligible []int64
	for rows.Next() {
		var uid int64
		var bound, edited bool
		if err := rows.Scan(&uid, &bound, &edited); err != nil {
			rows.Close()
			return nil, err
		}
		all = append(all, uid)
		if !bound && !edited {
			eligible = append(eligible, uid)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	switch {
	case len(eligible) > 1:
		return nil, refuse("several unbound accounts share the email")
	case len(eligible) == 1:
		res, err := tx.ExecContext(ctx,
			`UPDATE users SET googleSubject = ? WHERE userId = ? AND googleSubject IS NULL`, id.Subject, eligible[0])
		if err != nil {
			return nil, err
		}
		if n, err := res.RowsAffected(); err != nil || n != 1 {
			if err == nil {
				err = fmt.Errorf("binding of user %d affected %d rows", eligible[0], n)
			}
			return nil, err
		}
		return queryGoogleUser(ctx, tx, `userId = ?`, eligible[0])
	case len(all) > 0:
		return nil, refuse("email belongs to an account that cannot be bound to this subject")
	}

	name := id.Name
	if name == "" {
		name = email
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO users (userUsername, userPassword, userName, userEmail, userIsAdmin, userIsExternal, googleSubject) VALUES (?, '', ?, ?, 0, 1, ?)`,
		email, name, email, id.Subject)
	if err != nil {
		return nil, err
	}
	uid, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return queryGoogleUser(ctx, tx, `userId = ?`, uid)
}

// queryGoogleUser reads the stored account; role and profile come from the
// database, never from the token.
func queryGoogleUser(ctx context.Context, tx *sql.Tx, where string, arg any) (*models.User, error) {
	var (
		userID   int64
		password sql.NullString
		username sql.NullString
		name     sql.NullString
		mail     sql.NullString
		admin    sql.NullInt64
	)
	err := tx.QueryRowContext(ctx,
		`SELECT userId, userPassword, userUsername, userName, userEmail, userIsAdmin FROM users WHERE `+where+` LIMIT 1`, arg).
		Scan(&userID, &password, &username, &name, &mail, &admin)
	if err != nil {
		return nil, err
	}
	return &models.User{
		UserID:       &userID,
		UserPassword: password.String,
		UserUsername: username.String,
		UserName:     name.String,
		UserEmail:    mail.String,
		UserIsAdmin:  admin.Valid && admin.Int64 != 0,
	}, nil
}
