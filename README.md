# Portfolio Manager API

A production-grade Go REST API for managing investment portfolios. Built with Gin framework, PostgreSQL, and JWT authentication.

## Features

- **User Management**: Secure registration and login with JWT tokens
- **Portfolio Management**: Create and manage multiple investment portfolios
- **Asset Tracking**: Add, update, and delete assets in portfolios
- **Performance Analytics**: Calculate portfolio performance metrics (gains, returns, asset allocation)
- **RESTful API**: Clean, well-documented endpoints
- **Production Ready**: Comprehensive error handling, logging, and testing

## Architecture

```
Client Request → Router → Middleware (Auth) → Handler → Database
   ↓              ↓           ↓              ↓         ↓
  HTTP     Gin Router   JWT Validation  Business  PostgreSQL
                                        Logic
```

## Getting Started

### Prerequisites

- Go 1.21+
- PostgreSQL 12+
- Docker (optional)

### Local Development

1. **Clone the repository**
   ```bash
   git clone https://github.com/striver-21/portfolio-manager.git
   cd portfolio-manager
   ```

2. **Set up PostgreSQL database**
   ```bash
   # Create database
   createdb portfolio_manager
   ```

3. **Configure environment**
   ```bash
   export DATABASE_URL="host=localhost user=postgres password=postgres dbname=portfolio_manager port=5432 sslmode=disable"
   export PORT=8080
   ```

4. **Install dependencies and run**
   ```bash
   go mod download
   go run .
   ```

Server will start at `http://localhost:8080`

### Docker Deployment

```bash
docker-compose up -d
```

## API Endpoints

### Authentication

- `POST /api/auth/register` - Register a new user
- `POST /api/auth/login` - Login and get JWT token

### Portfolios

- `POST /api/portfolios` - Create portfolio
- `GET /api/portfolios` - List all portfolios
- `GET /api/portfolios/{id}` - Get portfolio details
- `PUT /api/portfolios/{id}` - Update portfolio
- `DELETE /api/portfolios/{id}` - Delete portfolio
- `GET /api/portfolios/{id}/stats` - Get portfolio statistics

### Assets

- `POST /api/portfolios/{portfolio_id}/assets` - Add asset
- `PUT /api/portfolios/{portfolio_id}/assets/{asset_id}` - Update asset
- `DELETE /api/portfolios/{portfolio_id}/assets/{asset_id}` - Delete asset

### Health

- `GET /health` - Health check

## Usage Examples

### Register

```bash
curl -X POST http://localhost:8080/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "user@example.com",
    "password": "securepassword123"
  }'
```

Response:
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "email": "user@example.com"
}
```

### Login

```bash
curl -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "user@example.com",
    "password": "securepassword123"
  }'
```

Response:
```json
{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "user_id": "550e8400-e29b-41d4-a716-446655440000",
  "email": "user@example.com"
}
```

### Create Portfolio

```bash
curl -X POST http://localhost:8080/api/portfolios \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <TOKEN>" \
  -d '{
    "name": "My Portfolio",
    "description": "My investment portfolio"
  }'
```

### Add Asset

```bash
curl -X POST http://localhost:8080/api/portfolios/{portfolio_id}/assets \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <TOKEN>" \
  -d '{
    "symbol": "AAPL",
    "quantity": 10,
    "purchase_price": 150.00,
    "current_price": 175.00
  }'
```

### Get Portfolio Stats

```bash
curl http://localhost:8080/api/portfolios/{portfolio_id}/stats \
  -H "Authorization: Bearer <TOKEN>"
```

Response:
```json
{
  "total_value": 4250,
  "total_cost": 3500,
  "total_gain": 750,
  "total_gain_percent": 21.43,
  "asset_count": 2,
  "asset_breakdown": {
    "AAPL": 1500,
    "GOOGL": 2750
  }
}
```

## Testing

Run all tests:
```bash
go test -v -race -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

Run specific test:
```bash
go test -v -run TestCreatePortfolio
```

## Code Quality

- **Go vet**: Static analysis
  ```bash
  go vet ./...
  ```

- **Lint**: Code linting
  ```bash
  golangci-lint run
  ```

- **Tests**: Comprehensive unit tests with 70%+ coverage
  ```bash
  go test -v -coverprofile=coverage.out ./...
  ```

## Project Structure

```
portfolio-manager/
├── main.go              # Application entry point
├── models.go            # Data models
├── handlers.go          # HTTP handlers
├── handlers_test.go     # Handler tests
├── database.go          # Database operations
├── auth.go              # Authentication logic
├── middleware.go        # HTTP middleware
├── Dockerfile           # Docker configuration
├── docker-compose.yml   # Docker Compose setup
├── go.mod               # Go modules
├── README.md            # This file
└── .gitignore           # Git ignore rules
```

## Technologies

- **Go 1.21** - Programming language
- **Gin Web Framework** - HTTP server
- **GORM** - ORM for database operations
- **PostgreSQL** - Relational database
- **JWT** - Token-based authentication
- **Testify** - Testing utilities
- **Docker** - Containerization

## Future Enhancements

- [ ] Integrate real-time stock pricing API
- [ ] Advanced analytics (Sharpe ratio, volatility analysis)
- [ ] Portfolio rebalancing recommendations
- [ ] Transaction history tracking
- [ ] Export portfolio data to CSV/PDF
- [ ] Multi-currency support
- [ ] WebSocket for real-time updates

## Contributing

Contributions are welcome! Please follow these steps:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Security

⚠️ **Important**: Change the JWT secret key in production!

Update `jwtSecret` in `auth.go` or set via environment variable:
```bash
export JWT_SECRET="your-very-secure-secret-key-here"
```

## Support

For issues or questions, please open an GitHub issue.

## Author

**Sachin Soni** - [@striver-21](https://github.com/striver-21)
