package jwt

import (
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

func ParseToken(tokenString string, secret string) (int64, error) {
	// Парсим токен, передавая функцию, которая возвращает секретный ключ для проверки подписи
	// !!!!!передаём в PArse сам токен(tokenString) и метод keyFunc в Parse. return []byte(secret) - за счёт keyFunc возвращаем методу Parse секрет, который нужен ему, что бы он смог проверить подпись
	// !!!сам keyFunc получает входной параметр token, что бы проверить нужным ли нам алгоритмом подписан jwt токен, нам нужен HMAC
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// !!Обязательно проверяем, что алгоритм подписи тот, который мы ожидаем (HMAC)
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		return []byte(secret), nil
	})

	if err != nil {
		return 0, err
	}

	// Claims — это данные, которые кладуться внутри jwt
	// MapClaims потому что он позволяет обращаться к данным по ключу
	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		// Достаем ID пользователя. В JSON числа декодируются как float64, поэтому приводим к нему, а затем к int64.
		uidFloat, ok := claims["uid"].(float64)
		if !ok {
			return 0, errors.New("user_id (uid) not found in token")
		}

		return int64(uidFloat), nil
	}

	return 0, errors.New("invalid token")
}
