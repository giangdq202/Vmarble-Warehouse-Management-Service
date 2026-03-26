# GEMINI.md - Instructional Context for VWMS

Below are the core guidelines and conventions for maintaining and developing the **Vmarble Warehouse Management Service (VWMS)**.

## 1. Project Overview
- **Objective**: Backend system (Modular Monolith) for warehouse and production management in a woodworking factory.
- **Tech Stack**: 
  - Go 1.24 (Gin Gonic)
  - PostgreSQL 17 (pgxpool, goose migrations)
  - Swagger (API Documentation)
  - Docker Compose (Local Environment)
- **Architecture**: Modular Monolith. Each business domain resides in its own module under `internal/module/`. Modules communicate via Interfaces and must not directly import each other's source code.

## 2. Module Structure Conventions
Each module (e.g., `catalog`) must adhere to a 5-file structure:
1. `iface.go`: Defines the public `Service` interface, constants (enums), and DTOs (Data Transfer Objects) for input/output.
2. `service.go`: Contains Business Logic. Performs validation and orchestrates between the Repository and other dependencies.
3. `store.go`: Defines the internal (unexported) Repository interface (`store`).
4. `pgstore.go`: Implements the `store` interface using raw SQL with `pgx`. No business logic should reside here.
5. `handler.go`: Defines HTTP endpoints, performs data binding, and returns JSON responses.

*Note*: If a module depends on another module, define those dependencies in a `deps.go` file within that module.

## 3. Development Workflow & Basic Commands
- **Start Dev Environment**: `make dev` (Starts Docker + Migrations + Server).
- **Run Tests**: `make test`.
- **Update API Docs**: `make swagger` (Always run after changing handlers or DTOs).
- **Create New Migration**: `make migrate-create` (Enter the migration name when prompted).
- **Quality Check**: `make lint`.

## 4. Coding Conventions
- **Error Handling**:
  - Use sentinel errors defined in `internal/domain/errors.go` (e.g., `domain.ErrNotFound`).
  - Use `domain.NewBizError(sentinel, "message")` in the `Service` layer to provide detailed error information.
  - Use `httpkit.Error(c, err)` in the `Handler` layer to automatically map Go errors to their corresponding HTTP status codes.
- **Validation**: Always perform input validation within the `Service` layer.
- **Database**: 
  - DO NOT use an ORM. Use raw SQL.
  - Use `context.Context` for all DB queries.
  - Use `pgxpool` for connection management.
- **Naming**: 
  - `ID` (not `Id`).
  - DB table/column names: `snake_case`.
  - JSON tags: `snake_case`.

## 5. Database & API Strategy on Staging/Prod (Security)
- **Zero Public Ports**: No ports (except SSH 22) are exposed to the public Internet on Staging.
- **Mandatory SSH Tunnel**: Access to both **PostgreSQL (5432)** and **Backend API (8080)** is only allowed via SSH Tunnel.
- **SSH Config Example**:
  ```ssh
  Host staging
      HostName 163.61.182.158
      User root
      LocalForward 5432 127.0.0.1:5432
      LocalForward 8080 127.0.0.1:8080
  ```
- **Fix-forward Migration**: Never use `migrate-down` on Staging or Production. All changes must be handled by creating a new migration file.

## 6. Collaboration Conventions
### Branch Naming
Use short prefixes followed by a descriptive name in `kebab-case`:
- `feat/` : New features (e.g., `feat/add-board-cutting`)
- `fix/` : Bug fixes (e.g., `fix/stock-calculation`)
- `docs/` : Documentation changes
- `chore/` : Maintenance tasks, configuration updates
- `test/` : Adding or fixing tests
- `refactor/` : Code refactoring without behavioral changes

### Commit Messages
Follow **Conventional Commits** (use lowercase for type):
- `feat: add ability to track remnants`
- `fix: correct area calculation logic`
- `docs: update deployment runbook`
- `chore: update dependencies`

### Pull Request Guidelines
- **Target Branch**: Always target `dev` for feature branches. Merge `dev` into `main` only for releases.
- **Description Template**:
  - **Summary**: What does this PR do?
  - **Changes**: Bullet points of key modifications.
  - **Testing**: How was this verified? (e.g., "Ran `make test`", "Manual check via Swagger").
- **Review**: `main` requires at least 1 approval. `dev` allows self-merge for speed but internal review is encouraged.

## 7. Development Workflow
1. Create a feature branch from `dev` (e.g., `feat/new-feature`).
2. Implement code and write tests.
3. Run `make swagger` if there are API changes.
4. Push code and create a Pull Request to `dev`.
5. CD will automatically trigger upon merging into `dev`.

## 8. Important Reference Documents
- Detailed Architecture: `docs/architecture.md`
- Warehouse/Costing Logic: `docs/backend-business-logic-vi.md`
- CD Runbook: `docs/runbooks/staging-cd.md`
- Contributor Guidelines: `CLAUDE.md`
