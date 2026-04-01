# API Reference

## User Object

| Field | Type | Description |
|-------|------|-------------|
| id | int | Auto-incrementing identifier |
| name | string | Display name |
| email | string | Email address |
| created_at | string | RFC3339 timestamp |

## Create User

**POST** `/api/users`

Request body:
```json
{
  "name": "Alice",
  "email": "alice@example.com"
}
```

Response (201):
```json
{
  "id": 1,
  "name": "Alice",
  "email": "alice@example.com",
  "created_at": "2024-01-15T10:30:00Z"
}
```

## List Users

**GET** `/api/users`

Response (200):
```json
[
  {"id": 1, "name": "Alice", "email": "alice@example.com", "created_at": "..."}
]
```

## Get User

**GET** `/api/users/:id`

Response (200): Single user object.
Response (404): `{"error": "not found"}`
