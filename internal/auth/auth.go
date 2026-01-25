package auth
import "errors"
type User struct {
    ID       int
    Username string
    Password string 
}
type Service struct {
}
func NewService() *Service {
    return &Service{}
}
func (s *Service) Login(username, password string) (string, error) {
    if username == "" || password == "" {
        return "", errors.New("invalid credentials")
    }
    return "valid-jwt-token", nil
}
