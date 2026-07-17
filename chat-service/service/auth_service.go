package service

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"ws-ex/model"
)

type AuthService struct {
	db        *gorm.DB
	jwtSecret []byte
}

func NewAuthService(db *gorm.DB, jwtSecret string) *AuthService {
	return &AuthService{
		db:        db,
		jwtSecret: []byte(jwtSecret),
	}
}

func (s *AuthService) Register(username, password string) (*model.User, error) {
	var existing model.User
	if err := s.db.Where("username = ?", username).First(&existing).Error; err == nil {
		return nil, errors.New("username already exists")
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := model.User{
		Username: username,
		Password: string(hashed),
		Balance:  InitialBalance(),
	}
	if err := s.db.Create(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *AuthService) Login(username, password string) (string, *model.User, error) {
	var user model.User
	if err := s.db.Where("username = ?", username).First(&user).Error; err != nil {
		return "", nil, errors.New("invalid username or password")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return "", nil, errors.New("invalid username or password")
	}

	token, err := s.generateToken(user.ID, user.Username)
	if err != nil {
		return "", nil, err
	}
	return token, &user, nil
}

func (s *AuthService) generateToken(userID uint, username string) (string, error) {
	claims := jwt.MapClaims{
		"user_id":  userID,
		"username": username,
		"exp":      time.Now().Add(72 * time.Hour).Unix(),
		"iat":      time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

func (s *AuthService) ValidateToken(tokenString string) (uint, string, error) {
	token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return s.jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return 0, "", errors.New("invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return 0, "", errors.New("invalid claims")
	}

	userIDFloat, ok := claims["user_id"].(float64)
	if !ok {
		return 0, "", errors.New("invalid user_id in token")
	}
	username, _ := claims["username"].(string)
	return uint(userIDFloat), username, nil
}

// GetUser returns a user by id.
func (s *AuthService) GetUser(userID uint) (*model.User, error) {
	var user model.User
	if err := s.db.First(&user, userID).Error; err != nil {
		return nil, errors.New("user not found")
	}
	return &user, nil
}

// UpdateProfile updates username and optionally password. Returns a fresh JWT.
func (s *AuthService) UpdateProfile(userID uint, username, newPassword, currentPassword string) (string, *model.User, error) {
	var user model.User
	if err := s.db.First(&user, userID).Error; err != nil {
		return "", nil, errors.New("user not found")
	}

	// Changing password requires current password.
	if newPassword != "" {
		if currentPassword == "" {
			return "", nil, errors.New("current password is required to set a new password")
		}
		if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(currentPassword)); err != nil {
			return "", nil, errors.New("current password is incorrect")
		}
		hashed, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
		if err != nil {
			return "", nil, err
		}
		user.Password = string(hashed)
	}

	// Username change uniqueness check.
	if username != user.Username {
		var existing model.User
		if err := s.db.Where("username = ? AND id <> ?", username, userID).First(&existing).Error; err == nil {
			return "", nil, errors.New("username already exists")
		}
		user.Username = username
	}

	if err := s.db.Save(&user).Error; err != nil {
		return "", nil, err
	}

	token, err := s.generateToken(user.ID, user.Username)
	if err != nil {
		return "", nil, err
	}
	return token, &user, nil
}
