# PulsePoll

PulsePoll is a live polling app built with React, Go/Gin, MongoDB, and Redis. Users sign up, create a poll, share its URL, and watch vote counts update live.

## Stack and data flow

- React/Vite renders the application and listens to the poll's server-sent event stream.
- Go/Gin validates requests, authenticates poll creators with JWT, and owns all database access.
- MongoDB persists users, polls, and individual vote records. A unique `(poll_id, voter_key)` index prevents repeat votes from the same browser key.
- Redis stores per-option vote counters and publishes each accepted vote. SSE handlers subscribe to poll-specific Redis channels so connected viewers receive updates immediately.
- If a poll's Redis counters are missing, the backend rebuilds that poll's totals from MongoDB when the poll is first opened. This avoids scanning every poll during each backend startup.
- Poll pages show a live **Room Pulse** insight: a warming-up state, a clear signal, an emerging front-runner, or a split room.
- Signed-in creators can open **My Polls** to revisit their saved polls and current results; poll and vote records are stored in MongoDB.
- Login sessions last 30 days. If a saved token is no longer accepted, the app clears it and guides the user back to login.

Redis and MongoDB are required for the backend to start; it does not silently fall back to memory.

## Local development setup

The backend needs both a reachable MongoDB server and a reachable Redis server. MongoDB may be local or hosted. Redis can be local for development or hosted on Redis Cloud.

For the current Windows development setup, MongoDB is running locally. The backend env file is configured for `mongodb://localhost:27017` and `redis://localhost:6379`. A Redis-compatible service such as Memurai must be installed and running on port 6379 before the backend can start. In an **Administrator PowerShell** window, install Memurai with:

```powershell
choco install memurai-developer --yes --no-progress
Get-Service *Memurai*
Start-Service Memurai
Test-NetConnection 127.0.0.1 -Port 6379
```

The last command should return `True`. Starting the app without a Redis service is expected to fail; this is intentional, not a React or MongoDB error.

For hosted services:

1. Create a MongoDB Atlas cluster and database user. In Atlas Network Access, allow your development IP. Copy the connection string from **Connect > Drivers**.
2. Create a Redis Cloud database and copy its public endpoint, port, and password. Use the TLS URL form `rediss://default:<password>@<host>:<port>`.
3. In `backend\.env`, replace `MONGO_URI` and `REDIS_URL` with the service connection URLs. URL-encode special characters in usernames/passwords.

Keep `backend\.env` private and do not commit it; it is ignored by Git. Generate a new JWT secret with:

```powershell
node -e "console.log(require('crypto').randomBytes(32).toString('hex'))"
```

Use `redis://` for a local Redis-compatible service. Use `rediss://` for Redis Cloud's TLS connection.

The frontend uses the hosted PulsePoll API by default, including when opened from a local Vite preview. To develop against a local backend instead, set `VITE_API_URL=http://localhost:8080/api` in `frontend/.env`; that backend must have reachable MongoDB and Redis services. For another deployment, set `VITE_API_URL` to that backend's API URL.

## Run locally

Install Node.js. You can run the frontend against the hosted API without installing Go, MongoDB, or Redis. To run the backend locally as well, install Go 1.25 or newer and make sure MongoDB and Redis are reachable using `backend\.env`.

To start the frontend, open PowerShell from the project root:

```powershell
cd frontend
npm install
npm run dev
```

Open the localhost URL printed by Vite (commonly `http://localhost:5173`). The local preview uses the hosted API, so your account and polls are saved online. To run the backend locally, set `VITE_API_URL` in `frontend/.env`, start MongoDB and Redis, then run `cd backend; go run .` in another terminal.

The local preview's address itself cannot be opened by a phone. Its poll page provides the corresponding hosted link; use that link on both laptop and phone to see the same poll and live results. The hosted poll page has a **Copy poll link** button and a separate **More options** share action.

## API overview

- `POST /api/auth/signup` and `POST /api/auth/login`
- `POST /api/polls` (JWT required), `GET /api/polls`, `GET /api/polls/:id`
- `POST /api/polls/:id/vote`
- `GET /api/polls/:id/stream` (SSE fed by Redis Pub/Sub)
- `GET /health` checks MongoDB and Redis connectivity

The browser creates a persistent random voter key in local storage. This prevents accidental duplicate votes from the same browser, but it is not identity verification and can be bypassed by a determined visitor.

## Before deployment

Set the production backend environment variables (`PORT`, `JWT_SECRET`, `MONGO_URI`, `MONGO_DATABASE`, `REDIS_URL`, `FRONTEND_ORIGIN`) and the frontend `VITE_API_URL`. Restrict Atlas network access to the backend host where possible. Never expose database credentials in frontend variables or commit them to the repository. Test the deployed share link and live updates in separate browsers.

The required 3–5 minute submission video is mandatory. Explain the app, the hardest challenge and how you solved it, and whether/how you used AI tools. Make sure you understand the MongoDB persistence, Redis counter/Pub/Sub flow, JWT authentication, and SSE well enough to discuss them in interviews.
