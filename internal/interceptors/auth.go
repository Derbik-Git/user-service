// это просто пример, что бы код скоомпилировался, так это всё будет происходить с реальным токеном, через библиотеку golang-jwt/jwt
package interceptors // интерцептор - пакет, занимающийся чтением методанных
import (
	"context"
	"strings"

	"github.com/Derbik-Git/user-service/internal/lib/jwt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type contextKey string

const UserIDKey contextKey = "user_id"

func AuthInterceptor(secret string) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context, // контекст запроса(в нём лежат сами метаданные)
		req any, // тело запроса
		info *grpc.UnaryServerInfo, // информаци о том, куда идёт запрос(имя метода)
		handler grpc.UnaryHandler, // конечный метод в хендлере (например, GetUser). Вызов handler(...) - это разрешение "проходи!"
	) (any, error) {
		// в начале должны проверить куда идёт запрос, так как при регистртации user, проверка токена не требуеться(пользователь только создался)
		if info.FullMethod == "/user.v1.UserService/CreateUser" {
			return handler(ctx, req) // разрешаем пройти без проверки, если запрос был, на регистрацию пользовтаеля
		}

		// теперь нужно извлечь из контекста токен
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "metadata is required")
		}

		// ищем заголовок "authorization" в metadata
		authHeader := md.Get("authorization")
		if len(authHeader) == 0 {
			return nil, status.Error(codes.Unauthenticated, "authorization header is missing")
		}

		// Заголовок обычно выглядит так: "Bearer секретный_токен".
		// Нам нужно отрезать слово "Bearer " чтобы получить сам токен.
		tokenStr := strings.TrimSpace(strings.TrimPrefix(authHeader[0], "Bearer"))

		// нужно проверить сам токен
		userID, err := jwt.ParseToken(tokenStr, secret)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, "invalid token")
		}

		// если токен прошёл проверку, выдаём новому контексту, новый ID пользователя
		newCtx := context.WithValue(ctx, UserIDKey, userID)

		// если всё нормально, передаём управление дальше, хендлеру
		return handler(newCtx, req)
	}
}

func validateToken(token string) (int64, error) {
	if token == "ban" {
		return 0, status.Error(codes.Unauthenticated, "user is bunned")
	}

	return 1, nil
}
