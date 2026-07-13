package models

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

type User struct {
	UserID           int `json:"userID,string"`
	PlayerID         sql.NullInt32
	PlayerName       sql.NullString
	Email            string
	HashedPassword   []byte
	Created          time.Time
	LastLogin        sql.NullTime
	Active           bool
	IsAdmin          bool
	VerificationHash []byte
}

type UserModel struct {
	DB *sql.DB
}

// AuthContext holds all per-request auth flags returned by the single
// consolidated GetAuthContext query.
type AuthContext struct {
	Active   bool
	IsAdmin  bool
	PlayerID sql.NullInt32
	UserName string // always non-empty: player name or email
}

func (m *UserModel) Insert(email, password string) (int, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return 0, err
	}

	verificationHash, err := bcrypt.GenerateFromPassword([]byte(email+password), 12)
	if err != nil {
		return 0, err
	}

	statement := `INSERT INTO users (email, hashed_password, created, active, verification_hash)
    VALUES(?, ?, UTC_TIMESTAMP(), false, ?)`

	result, err := m.DB.Exec(statement, email, string(hashedPassword), string(verificationHash))
	if err != nil {
		var mySQLError *mysql.MySQLError
		if errors.As(err, &mySQLError) {
			if mySQLError.Number == 1062 && strings.Contains(mySQLError.Message, "email") {
				return 0, ErrDuplicateEmail
			}
		}
		return 0, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	return int(id), nil
}

func (m *UserModel) Delete(userID int) error {

	statement := "delete from userRole where userID = ?"

	_, err := m.DB.Exec(statement, userID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}

	statement = "delete from users where id = ?"

	_, err = m.DB.Exec(statement, userID)

	return err
}

func (m *UserModel) ResetPassword(email, password string) error {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return err
	}

	statement := `UPDATE users set hashed_password = ? where email = ?;`

	_, err = m.DB.Exec(statement, string(hashedPassword), email)

	return err
}

func (m *UserModel) GetUser(id int) (*User, error) {

	stmt := `select u.id as userid,
				p.id as playerid,
				CONCAT(p.firstname, " ", p.lastname) as playername,
				u.email,
				u.created,
				u.lastlogin,
				u.active,
				exists(Select 1 from userRole ur where ur.userID = u.id and ur.roleID = "ADMIN") as isAdmin,
				verification_hash
			from users u
			left join players p on p.id = u.playerID
			where u.id = ?;`

	user := &User{}
	err := m.DB.QueryRow(stmt, id).Scan(&user.UserID, &user.PlayerID, &user.PlayerName, &user.Email, &user.Created, &user.LastLogin, &user.Active, &user.IsAdmin, &user.VerificationHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRecord
		}
		return nil, err
	}

	return user, nil
}

func (m *UserModel) GetUserByEmail(email string) (*User, error) {

	stmt := `select u.id as userid,
				p.id as playerid,
				CONCAT(p.firstname, " ", p.lastname) as playername,
				u.email,
				u.created,
				u.lastlogin,
				u.active,
				exists(Select 1 from userRole ur where ur.userID = u.id and ur.roleID = "ADMIN") as isAdmin
			from users u
			left join players p on p.id = u.playerID
			where u.email = ?;`

	user := &User{}
	err := m.DB.QueryRow(stmt, email).Scan(&user.UserID, &user.PlayerID, &user.PlayerName, &user.Email, &user.Created, &user.LastLogin, &user.Active, &user.IsAdmin)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRecord
		}
		return nil, err
	}
	return user, nil
}

func (m *UserModel) Authenticate(email, password string) (int, error) {
	var id int
	var hashedPassword []byte

	stmt := "SELECT id, hashed_password FROM users WHERE email = ?"

	err := m.DB.QueryRow(stmt, email).Scan(&id, &hashedPassword)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrInvalidCredentials
		} else {
			return 0, err
		}
	}

	err = bcrypt.CompareHashAndPassword(hashedPassword, []byte(password))
	if err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return 0, ErrInvalidCredentials
		} else {
			return 0, err
		}
	}

	//authenticated, so update last login
	err = m.updateLastLogin(id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (m *UserModel) updateLastLogin(userID int) error {
	statement := "update users set lastlogin = ? where id = ?;"

	_, err := m.DB.Exec(statement, time.Now(), userID)

	return err
}

func (m *UserModel) UpdateEmail(userID int, email string) error {
	statement := "update users set email = ? where id = ?;"

	_, err := m.DB.Exec(statement, email, userID)

	return err
}

func (m *UserModel) Exists(id int) (bool, error) {
	var exists bool

	stmt := "SELECT EXISTS(SELECT true FROM users WHERE id = ?)"

	err := m.DB.QueryRow(stmt, id).Scan(&exists)
	return exists, err
}

func (m *UserModel) Active(id int) (bool, error) {
	var active bool

	stmt := "SELECT active FROM users WHERE id = ?"

	err := m.DB.QueryRow(stmt, id).Scan(&active)
	return active, err
}

func (m *UserModel) GetByVerificationHash(hash string) (*User, error) {
	user := &User{}

	stmt := "SELECT id, email FROM users WHERE verification_hash = ?"

	err := m.DB.QueryRow(stmt, hash).Scan(&user.UserID, &user.Email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return user, ErrNoRecord
		} else {
			return user, err
		}
	}
	return user, err
}

// GetAuthContext fetches all authentication context for a user in a single
// query: active flag, admin flag, and their linked player (if any).
func (m *UserModel) GetAuthContext(id int) (*AuthContext, error) {
	stmt := `SELECT
		u.active,
		EXISTS(SELECT 1 FROM userRole WHERE userID = u.id AND roleID = 'ADMIN'),
		p.id,
		COALESCE(CONCAT(p.firstname, ' ', p.lastname), u.email)
	FROM users u
	LEFT JOIN players p ON p.id = u.playerID
	WHERE u.id = ?`

	ac := &AuthContext{}
	err := m.DB.QueryRow(stmt, id).Scan(
		&ac.Active,
		&ac.IsAdmin,
		&ac.PlayerID,
		&ac.UserName,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoRecord
		}
		return nil, err
	}
	return ac, nil
}

func (m *UserModel) GetVerificationHashByEmail(email string) (string, error) {
	var hash string

	stmt := "SELECT verification_hash FROM users WHERE email = ?"

	err := m.DB.QueryRow(stmt, email).Scan(&hash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return hash, ErrNoRecord
		} else {
			return hash, err
		}
	}
	return hash, err
}

func (m *UserModel) Activate(id int) error {
	statement := `UPDATE users SET active = 1 where id = ?`

	_, err := m.DB.Exec(statement, id)

	return err
}

// SetPlayerID links a user account to a roster player record.
func (m *UserModel) SetPlayerID(userID, playerID int) error {
	statement := `UPDATE users SET playerID = ? WHERE id = ?`

	_, err := m.DB.Exec(statement, playerID, userID)

	return err
}

func (m *UserModel) InsertUserRole(userID int, roleCode string) error {
	statement := "insert into userRole (userID, roleID) values (?, ?)"

	_, err := m.DB.Exec(statement, userID, roleCode)

	return err
}

func (m *UserModel) DeleteUserRole(userID int, roleCode string) error {
	statement := "delete from userRole where userID = ? and roleID = ?"

	_, err := m.DB.Exec(statement, userID, roleCode)

	return err
}

func (m *UserModel) UserHasRole(userID int, roleCode string) (bool, error) {
	statement := `select exists( select true
		from roles r
		join userRole ur on ur.roleID = r.code
		where ur.userID = ?
		and r.code = ?)`

	var exists bool
	err := m.DB.QueryRow(statement, userID, roleCode).Scan(&exists)
	return exists, err
}

type UserSearchCriteria struct {
	FirstName string
	LastName  string
	Email     string
	Sort      string
	Order     string
	Offset    int
	Limit     int
}

type UserSearchResult struct {
	UserID     int
	PlayerID   sql.NullInt32
	PlayerName sql.NullString
	Email      string
	Created    time.Time
	LastLogin  sql.NullTime
	Active     bool
	IsAdmin    bool
	FullCount  sql.NullInt32
}

func (m *UserModel) Search(criteria *UserSearchCriteria) ([]*UserSearchResult, error) {
	statement, queryParams := buildUserSearchStatement(criteria)
	rows, err := m.DB.Query(statement, queryParams...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := []*UserSearchResult{}
	for rows.Next() {
		user := &UserSearchResult{}
		err := rows.Scan(&user.UserID, &user.PlayerID, &user.PlayerName, &user.Email,
			&user.Created, &user.LastLogin, &user.Active, &user.IsAdmin, &user.FullCount)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, nil
}

func buildUserSearchStatement(criteria *UserSearchCriteria) (string, []any) {
	allowedSorts := map[string]string{
		"playername": "playername",
		"email":      "u.email",
		"created":    "u.created",
		"lastlogin":  "u.lastlogin",
	}

	base := `SELECT
		u.id AS userid,
		p.id AS playerid,
		CONCAT(IFNULL(p.firstname, ''), ' ', IFNULL(p.lastname, '')) AS playername,
		u.email,
		u.created,
		u.lastlogin,
		u.active,
		EXISTS(SELECT 1 FROM userRole ur WHERE ur.userID = u.id AND ur.roleID = 'ADMIN') AS isAdmin,
		COUNT(*) OVER() AS fullCount
	FROM users u
	LEFT JOIN players p ON p.id = u.playerID
	WHERE 1=1`

	queryParams := []any{}
	where := ""

	if criteria.FirstName != "" {
		where += " AND UPPER(p.firstname) LIKE CONCAT('%', ?, '%')"
		queryParams = append(queryParams, strings.ToUpper(criteria.FirstName))
	}
	if criteria.LastName != "" {
		where += " AND UPPER(p.lastname) LIKE CONCAT('%', ?, '%')"
		queryParams = append(queryParams, strings.ToUpper(criteria.LastName))
	}
	if criteria.Email != "" {
		where += " AND UPPER(u.email) LIKE CONCAT('%', ?, '%')"
		queryParams = append(queryParams, strings.ToUpper(criteria.Email))
	}

	orderBy := "ORDER BY u.email ASC"
	if sortCol, ok := allowedSorts[criteria.Sort]; ok {
		order := "ASC"
		if criteria.Order == "DESC" {
			order = "DESC"
		}
		orderBy = fmt.Sprintf("ORDER BY %s %s", sortCol, order)
	}

	limit := fmt.Sprintf("LIMIT %d OFFSET %d", criteria.Limit, criteria.Offset)
	return fmt.Sprintf("%s%s %s %s;", base, where, orderBy, limit), queryParams
}

func (m *UserModel) List() ([]*User, error) {
	statement := `select u.id as userid,
		p.id as playerid,
		CONCAT(p.firstname, " ", p.lastname) as playername,
		u.email,
		u.created,
		u.lastlogin,
		u.active,
		exists(Select 1 from userRole ur where ur.userID = u.id and ur.roleID = "ADMIN") as isAdmin
	from users u
	left join players p on p.id = u.playerID;`

	rows, err := m.DB.Query(statement)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := []*User{}

	for rows.Next() {
		user := &User{}
		err := rows.Scan(&user.UserID, &user.PlayerID, &user.PlayerName, &user.Email, &user.Created, &user.LastLogin, &user.Active, &user.IsAdmin)
		if err != nil {
			return nil, err
		}

		users = append(users, user)
	}

	return users, nil
}

func (m *UserModel) ToggleActive(userID int) error {
	statement := "update users set active = !active where id = ?;"

	_, err := m.DB.Exec(statement, userID)

	return err
}
