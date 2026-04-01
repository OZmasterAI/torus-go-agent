# User API

A simple REST API for managing users.

## Endpoints

- `GET /` - Health check
- `GET /api/users` - List all users
- `POST /api/users` - Create a user (JSON body: `{"name": "...", "email": "..."}`)
- `GET /api/users/:id` - Get user by ID

## Running

```bash
go run src/*.go
```

## Testing

```bash
cd tests && go test -v
```
