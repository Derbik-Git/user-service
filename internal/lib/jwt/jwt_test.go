package jwt

import (
	"testing"
	"time"

	jwtgo "github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const secret = "test-secret-key"

// Вспомогательная функция для создания токена
func TestGenerateToken(t *testing.T, userID int64, secret string, ttl time.Duration) (string, error) {
	claims := jwtgo.MapClaims{
		"uid": float64(userID),
		"exp": time.Now().Add(ttl).Unix(),
	}
	token := jwtgo.NewWithClaims(jwtgo.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func TestParseToken(t *testing.T) {
	// 1. Успешный разбор валидного токена
	t.Run("valid token", func(t *testing.T) {
		tokenStr, err := TestGenerateToken(t, 123, secret, time.Hour)
		require.NoError(t, err)

		userID, err := ParseToken(tokenStr, secret)
		require.NoError(t, err)
		assert.Equal(t, int64(123), userID)
	})

	// 2. Ошибка при неверном секрете
	t.Run("invalid secret", func(t *testing.T) {
		tokenStr, err := TestGenerateToken(t, 123, "wrong-secret", time.Hour)
		require.NoError(t, err)

		_, err = ParseToken(tokenStr, secret)
		require.Error(t, err)
	})

	// 3. Ошибка при истекшем токене
	t.Run("expired token", func(t *testing.T) {
		tokenStr, err := TestGenerateToken(t, 123, secret, -time.Hour)
		require.NoError(t, err)

		_, err = ParseToken(tokenStr, secret)
		require.Error(t, err)
	})
}
