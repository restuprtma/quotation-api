# Graph Report - .  (2026-08-12)

## Corpus Check
- 191 files · ~112,709 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 1327 nodes · 2990 edges · 64 communities (57 shown, 7 thin omitted)
- Extraction: 83% EXTRACTED · 17% INFERRED · 0% AMBIGUOUS · INFERRED: 498 edges (avg confidence: 0.8)
- Token cost: 0 input · 0 output

## Community Hubs (Navigation)
- Core: internal_modules_quo
- Core: domain_apikey
- Core: service_approvalrequ
- Core: service_companyservi
- Core: audit_service_record
- Core: service_roleservice
- Core: repository_approvalc
- Core: service_userservice
- Core: repository_branchrep
- Core: internal_modules_cor
- Core: repository_userrepos
- Core: discord_notifier
- Core: internal_modules_cor
- Core: internal_modules_cor
- Core: pkg_email_smtp_newsm
- Core: internal_config_conf
- Core: repository_companyus
- Core: service_userbranchse
- Core: repository_refreshto
- Core: repository_companyre
- Core: service_authservice
- Core: internal_modules_cor
- Core: repository_rolerepos
- Core: internal_middleware_
- Core: types_date
- Core: handler_branchhandle
- Core: repository_userbranc
- Core: pkg_logger_logger
- Core: authz_service
- Core: dto_companyinfo
- Core: middleware_ratelimit
- Core: storage_gcsclient
- Core: internal_router_rout
- Core: firebase_verifiedtok
- Core: handler_authhandler
- Core: handler_userhandler
- Core: internal_modules_cor
- Core: query_querybuilder
- Core: repository_userident
- Core: email_noopemailservi
- Core: internal_modules_cor
- Core: internal_modules_quo
- Core: internal_middleware_
- Core: internal_middleware_
- Core: email_smtpemailservi
- Core: handler_approvalrequ
- Core: internal_middleware_
- Core: internal_middleware_
- Core: internal_middleware_
- Core: internal_modules_cor
- Core: pkg_token_generator
- Core: pkg_jwt_jwt
- Core: internal_shared_resp
- Core: jwt_claims
- Core: pkg_utils_time
- Core: pkg_validator_passwo
- Core: internal_middleware_
- Core: deployments_docker_d
- Core: domain_client
- Core: dto_mycompanyrespons
- Core: dto_logoutrequest
- Core: pkg_venturo_skeleton

## God Nodes (most connected - your core abstractions)
1. `Err()` - 145 edges
2. `Success()` - 69 edges
3. `AuthService` - 38 edges
4. `MustGetUserID()` - 37 edges
5. `CompanyService` - 33 edges
6. `GetCompanyID()` - 28 edges
7. `GetUserFromContext()` - 25 edges
8. `Info()` - 25 edges
9. `UserRepository` - 23 edges
10. `UserService` - 23 edges

## Surprising Connections (you probably didn't know these)
- `main()` --calls--> `Load()`  [INFERRED]
  cmd/api/main.go → internal/config/config.go
- `main()` --calls--> `ValidateSecret()`  [INFERRED]
  cmd/api/main.go → pkg/jwt/jwt.go
- `main()` --calls--> `Info()`  [INFERRED]
  cmd/api/main.go → pkg/logger/logger.go
- `logPoolStats()` --calls--> `Info()`  [INFERRED]
  internal/database/database.go → pkg/logger/logger.go
- `logPoolStats()` --calls--> `Int32()`  [INFERRED]
  internal/database/database.go → pkg/logger/logger.go

## Import Cycles
- None detected.

## Communities (64 total, 7 thin omitted)

### Community 0 - "Core: internal_modules_quo"
Cohesion: 0.06
Nodes (51): Contract, Quotation, QuotationItem, QuotationRevision, AIGenerateRequest, AIGenerateResponse, CreateQuotationItemRequest, CreateQuotationRequest (+43 more)

### Community 1 - "Core: domain_apikey"
Cohesion: 0.07
Nodes (29): ApiKeyModule, RoleRepository, ApiKey, ApiKeyEnvironment, ApiKeyListResponse, ApiKeyQueryParams, ApiKeyResponse, CreateApiKeyRequest (+21 more)

### Community 2 - "Core: service_approvalrequ"
Cohesion: 0.09
Nodes (30): ApprovalRequest, ApprovalRequestLevel, ApprovalByDocQueryParams, ApprovalRequestLevelResponse, ApprovalRequestQueryParams, ApprovalRequestResponse, ApproveRequest, CancelRequest (+22 more)

### Community 3 - "Core: service_companyservi"
Cohesion: 0.08
Nodes (28): CompanyModule, CompanyType, AddCompanyUserRequest, CompanyListResponse, CompanyResponse, CompanyTrashQueryParams, CompanyUserListResponse, CompanyUserQueryParams (+20 more)

### Community 4 - "Core: audit_service_record"
Cohesion: 0.05
Nodes (42): Actor, actorCtxKey, AuditLog, AuditLogResponse, Event, FieldChange, Handler, ListFilter (+34 more)

### Community 5 - "Core: service_roleservice"
Cohesion: 0.08
Nodes (27): PermissionLevel, Permissions, UserRole, AssignRoleRequest, CreateRoleRequest, RemoveRoleRequest, RoleListResponse, RoleQueryParams (+19 more)

### Community 6 - "Core: repository_approvalc"
Cohesion: 0.10
Nodes (27): Module, ApprovalConfig, ApprovalConfigLevel, ApprovalConfigLevelInput, ApprovalConfigLevelResponse, ApprovalConfigQueryParams, ApprovalConfigResponse, CreateApprovalConfigRequest (+19 more)

### Community 7 - "Core: service_userservice"
Cohesion: 0.11
Nodes (23): ChangePasswordRequest, CreateUserRequest, UpdateMeRequest, UpdateUserRequest, UserBranchItem, UserCompanyItem, UserListResponse, UserQueryParams (+15 more)

### Community 8 - "Core: repository_branchrep"
Cohesion: 0.10
Nodes (21): Branch, BranchListResponse, BranchQueryParams, BranchResponse, CreateBranchRequest, SyncUserBranchesRequest, UpdateBranchRequest, Time (+13 more)

### Community 9 - "Core: internal_modules_cor"
Cohesion: 0.09
Nodes (23): TranslationOverride, CreateTranslationOverrideRequest, PublicTranslationsResponse, TranslationOverrideListResponse, TranslationOverrideResponse, UpdateTranslationOverrideRequest, Time, Time (+15 more)

### Community 10 - "Core: repository_userrepos"
Cohesion: 0.12
Nodes (10): User, UserWithRoles, Time, Context, Pool, dbExec, Time, Tx (+2 more)

### Community 11 - "Core: discord_notifier"
Cohesion: 0.10
Nodes (22): CheckedEntry, Core, Event, Notifier, Entry, Level, discordCore, Mutex (+14 more)

### Community 12 - "Core: internal_modules_cor"
Cohesion: 0.12
Nodes (16): Module, Context, Handler, Service, NewHandler(), Handler, Pool, Repository (+8 more)

### Community 13 - "Core: internal_modules_cor"
Cohesion: 0.13
Nodes (19): CommandTag, ClientListResponse, ClientResponse, UpdateClientRequest, Time, collapseHyphens(), Client, Context (+11 more)

### Community 14 - "Core: pkg_email_smtp_newsm"
Cohesion: 0.13
Nodes (26): B, SMTPConfig, T, TestNoOpEmailService(), TestSMTPEmailService_ConfigInjection(), TestSMTPEmailService_ServiceInitialization(), TestSMTPEmailService_TemplateFiles(), TestSMTPEmailService_VerifyConfiguration() (+18 more)

### Community 15 - "Core: internal_config_conf"
Cohesion: 0.13
Nodes (24): AuthConfig, Config, DatabaseConfig, FirebaseConfig, GCSConfig, OpenAIConfig, RedisConfig, SecurityConfig (+16 more)

### Community 16 - "Core: repository_companyus"
Cohesion: 0.15
Nodes (10): CompanyUser, CompanyUserWithDetails, UserCompanyDetail, Time, Context, Pool, dbExec, Tx (+2 more)

### Community 17 - "Core: service_userbranchse"
Cohesion: 0.13
Nodes (14): BranchModule, UserBranchDetail, UserBranchHandler, Context, NewUserBranchHandler(), BranchRepository, Pool, Initialize() (+6 more)

### Community 18 - "Core: repository_refreshto"
Cohesion: 0.13
Nodes (10): AuthModule, RefreshToken, Time, Pool, RouterGroup, Initialize(), Context, Pool (+2 more)

### Community 19 - "Core: repository_companyre"
Cohesion: 0.23
Nodes (8): Company, CompanyQueryParams, Time, Context, Pool, NewCompanyRepository(), Err(), CompanyRepository

### Community 20 - "Core: service_authservice"
Cohesion: 0.16
Nodes (13): BranchRepository, RoleRepository, UserRepository, NewAuthService(), sanitizeUsername(), shortHex(), usernameFromEmail(), AuthService (+5 more)

### Community 21 - "Core: internal_modules_cor"
Cohesion: 0.18
Nodes (4): CompanyHandler, Context, Context, SuccessWithPagination()

### Community 22 - "Core: repository_rolerepos"
Cohesion: 0.21
Nodes (5): Role, Context, Pool, NewRoleRepository(), RoleRepository

### Community 23 - "Core: internal_middleware_"
Cohesion: 0.26
Nodes (7): ApiKeyHandler, ApprovalConfigHandler, GetCompanyID(), GetUserID(), Context, Context, NewApprovalConfigHandler()

### Community 24 - "Core: types_date"
Cohesion: 0.15
Nodes (12): Month, Time, NewDate(), ParseDate(), T, TestDate_MarshalJSON(), TestDate_RoundTrip(), TestDate_UnmarshalJSON() (+4 more)

### Community 25 - "Core: handler_branchhandle"
Cohesion: 0.20
Nodes (10): BranchHandler, BranchScope(), GetAllowedBranchIDs(), Context, HandlerFunc, IsBranchAllowed(), SetUserBranchResolver(), Context (+2 more)

### Community 26 - "Core: repository_userbranc"
Cohesion: 0.23
Nodes (8): UserBranch, Time, Context, Pool, Tx, NewUserBranchRepository(), userBranchExec, UserBranchRepository

### Community 27 - "Core: pkg_logger_logger"
Cohesion: 0.19
Nodes (15): generateKeyID(), Logger, Any(), Bool(), Debug(), Error(), Float64(), GetLogger() (+7 more)

### Community 28 - "Core: authz_service"
Cohesion: 0.28
Nodes (7): PermissionFetcher, Service, cacheKey(), Client, Context, Duration, NewService()

### Community 29 - "Core: dto_companyinfo"
Cohesion: 0.20
Nodes (11): ClientInfo, CompanyInfo, GoogleSignInResponse, MeResponse, SignInRequest, SignInResponse, SignUpRequest, SignUpResponse (+3 more)

### Community 30 - "Core: middleware_ratelimit"
Cohesion: 0.27
Nodes (11): CombinedRateLimiter(), Context, Duration, HandlerFunc, Time, IPBasedRateLimiter(), NewRateLimiter(), RateLimitMiddleware() (+3 more)

### Community 31 - "Core: storage_gcsclient"
Cohesion: 0.18
Nodes (9): Client, Context, Duration, NewGCSClient(), Reader, Client, FileInfo, GCSClient (+1 more)

### Community 32 - "Core: internal_router_rout"
Cohesion: 0.14
Nodes (10): main(), Engine, SetCompanyContextVerifier(), Service, SetAuthzService(), Pool, Setup(), CompanyContextVerifier (+2 more)

### Community 33 - "Core: firebase_verifiedtok"
Cohesion: 0.22
Nodes (9): GoogleSignInRequest, Client, VerifiedToken, marshalProfile(), truncateName(), Context, mapToken(), New() (+1 more)

### Community 34 - "Core: handler_authhandler"
Cohesion: 0.30
Nodes (5): AuthHandler, Context, NewAuthHandler(), GetDeviceInfoFromUserAgent(), Success()

### Community 35 - "Core: handler_userhandler"
Cohesion: 0.38
Nodes (5): UserHandler, MustGetUserID(), Context, NewUserHandler(), writeScopeError()

### Community 36 - "Core: internal_modules_cor"
Cohesion: 0.27
Nodes (6): DeviceInfo, RefreshTokenRequest, RefreshTokenResponse, containsRole(), Context, hashToken()

### Community 38 - "Core: repository_userident"
Cohesion: 0.24
Nodes (6): UserIdentity, Time, Context, Pool, NewUserIdentityRepository(), UserIdentityRepository

### Community 39 - "Core: email_noopemailservi"
Cohesion: 0.17
Nodes (3): EmailData, EmailService, NoOpEmailService

### Community 40 - "Core: internal_modules_cor"
Cohesion: 0.38
Nodes (4): Context, Handler, Service, NewHandler()

### Community 41 - "Core: internal_modules_quo"
Cohesion: 0.35
Nodes (5): Context, Handler, Service, NewHandler(), parsePagination()

### Community 42 - "Core: internal_middleware_"
Cohesion: 0.22
Nodes (5): RequirePermission(), RouterGroup, RouterGroup, RouterGroup, RouterGroup

### Community 43 - "Core: internal_middleware_"
Cohesion: 0.38
Nodes (9): findMatchingRole(), HandlerFunc, joinStrings(), RequireAllPermissions(), RequireAllRoles(), RequireAnyPermission(), RequireRole(), RouterGroup (+1 more)

### Community 45 - "Core: handler_approvalrequ"
Cohesion: 0.33
Nodes (3): ApprovalRequestHandler, Context, NewApprovalRequestHandler()

### Community 46 - "Core: internal_middleware_"
Cohesion: 0.20
Nodes (6): CompanyContext(), HandlerFunc, RouterGroup, RouterGroup, RouterGroup, RouterGroup

### Community 47 - "Core: internal_middleware_"
Cohesion: 0.36
Nodes (9): Context, HandlerFunc, handleApiKeyAuth(), JWTAuth(), OptionalAuth(), SetApiKeyValidator(), SetUserContext(), ApiKeyValidator (+1 more)

### Community 48 - "Core: internal_middleware_"
Cohesion: 0.38
Nodes (9): GetApiKeyID(), GetAuthType(), GetUserEmail(), GetUserRoles(), Context, IsApiKeyAuth(), MustGetUserFromContext(), SetApiKeyID() (+1 more)

### Community 49 - "Core: internal_modules_cor"
Cohesion: 0.42
Nodes (3): RoleHandler, GetUserFromContext(), Context

### Community 50 - "Core: pkg_token_generator"
Cohesion: 0.31
Nodes (5): GenerateBase64Token(), GenerateEmailVerificationToken(), GeneratePasswordResetToken(), GenerateRefreshToken(), GenerateSecureToken()

### Community 51 - "Core: pkg_jwt_jwt"
Cohesion: 0.46
Nodes (7): GenerateToken(), GetExpirationTime(), GetSecret(), Duration, ParseToken(), RefreshToken(), ValidateToken()

### Community 52 - "Core: internal_shared_resp"
Cohesion: 0.43
Nodes (6): Error(), Context, ValidationError(), Meta, Pagination, Response

### Community 54 - "Core: pkg_utils_time"
Cohesion: 0.33
Nodes (5): Location, FormatWhatsAppTime(), GetAsiaJakartaLocation(), Time, ParseUnixToWhatsAppTime()

### Community 55 - "Core: pkg_validator_passwo"
Cohesion: 0.48
Nodes (6): GetPasswordStrength(), isCommonPassword(), toLower(), ValidatePassword(), ValidatePasswordWithEmail(), PasswordStrength

### Community 56 - "Core: internal_middleware_"
Cohesion: 0.40
Nodes (3): HandlerFunc, RequireClientScope(), RouterGroup

### Community 57 - "Core: deployments_docker_d"
Cohesion: 0.83
Nodes (3): run_migrations(), docker-entrypoint.sh script, wait_for_db()

## Knowledge Gaps
- **15 isolated node(s):** `venturo-skeleton-go`, `UserRepository`, `ApproveRequest`, `RejectRequest`, `CancelRequest` (+10 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **7 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `Err()` connect `Core: repository_companyre` to `Core: internal_modules_quo`, `Core: domain_apikey`, `Core: service_approvalrequ`, `Core: service_companyservi`, `Core: audit_service_record`, `Core: service_roleservice`, `Core: repository_approvalc`, `Core: service_userservice`, `Core: repository_branchrep`, `Core: internal_modules_cor`, `Core: repository_userrepos`, `Core: internal_modules_cor`, `Core: repository_companyus`, `Core: repository_refreshto`, `Core: service_authservice`, `Core: repository_rolerepos`, `Core: repository_userbranc`, `Core: pkg_logger_logger`, `Core: authz_service`, `Core: dto_companyinfo`, `Core: firebase_verifiedtok`, `Core: internal_modules_cor`, `Core: repository_userident`, `Core: internal_middleware_`, `Core: internal_middleware_`, `Core: email_smtpemailservi`, `Core: internal_middleware_`?**
  _High betweenness centrality (0.486) - this node is a cross-community bridge._
- **Why does `Info()` connect `Core: internal_middleware_` to `Core: internal_router_rout`, `Core: internal_modules_quo`, `Core: domain_apikey`, `Core: service_approvalrequ`, `Core: firebase_verifiedtok`, `Core: internal_modules_cor`, `Core: service_companyservi`, `Core: service_roleservice`, `Core: repository_branchrep`, `Core: internal_middleware_`, `Core: email_smtpemailservi`, `Core: internal_config_conf`, `Core: pkg_logger_logger`, `Core: dto_companyinfo`, `Core: storage_gcsclient`?**
  _High betweenness centrality (0.141) - this node is a cross-community bridge._
- **Why does `Success()` connect `Core: handler_authhandler` to `Core: handler_userhandler`, `Core: internal_modules_cor`, `Core: internal_modules_quo`, `Core: internal_modules_cor`, `Core: handler_approvalrequ`, `Core: internal_modules_cor`, `Core: service_userbranchse`, `Core: internal_shared_resp`, `Core: internal_modules_cor`, `Core: internal_middleware_`, `Core: handler_branchhandle`?**
  _High betweenness centrality (0.088) - this node is a cross-community bridge._
- **Are the 143 inferred relationships involving `Err()` (e.g. with `.InsertTx()` and `.GetPermissions()`) actually correct?**
  _`Err()` has 143 INFERRED edges - model-reasoned connections that need verification._
- **Are the 67 inferred relationships involving `Success()` (e.g. with `.Create()` and `.GetByID()`) actually correct?**
  _`Success()` has 67 INFERRED edges - model-reasoned connections that need verification._
- **Are the 34 inferred relationships involving `MustGetUserID()` (e.g. with `.Create()` and `.Delete()`) actually correct?**
  _`MustGetUserID()` has 34 INFERRED edges - model-reasoned connections that need verification._
- **What connects `venturo-skeleton-go`, `UserRepository`, `ApproveRequest` to the rest of the system?**
  _15 weakly-connected nodes found - possible documentation gaps or missing edges._