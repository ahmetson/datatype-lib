// Dynamic credentials for the database
package vault

import "time"

type Environment struct {
	// The address of this service
	MyAddress string `               env:"MY_ADDRESS"                    default:":8080"                        description:"Listen to http traffic on this tcp address"             long:"my-address"`

	// Vault address, approle login credentials, and secret locations
	VaultAddress             string `env:"VAULT_ADDRESS"                 default:"localhost:8200"               description:"Vault address"                                          long:"vault-address"`
	VaultApproleRoleID       string `env:"VAULT_APPROLE_ROLE_ID"         required:"true"                        description:"AppRole RoleID to log in to Vault"                      long:"vault-approle-role-id"`
	VaultApproleSecretIDFile string `env:"VAULT_APPROLE_SECRET_ID_FILE"  default:"/tmp/secret"                  description:"AppRole SecretID file path to log in to Vault"          long:"vault-approle-secret-id-file"`
	VaultAPIKeyPath          string `env:"VAULT_API_KEY_PATH"            default:"api-key"                      description:"Path to the API key used by 'secure-service'"           long:"vault-api-key-path"`
	VaultAPIKeyMountPath     string `env:"VAULT_API_KEY_MOUNT_PATH"      default:"kv-v2"                        description:"The location where the KV v2 secrets engine has been mounted in Vault" long:"vault-api-key-mount-path"`
	VaultAPIKeyField         string `env:"VAULT_API_KEY_FIELD"           default:"api-key-field"                description:"The secret field name for the API key"                  long:"vault-api-key-descriptor"`
	VaultDatabaseCredsPath   string `env:"VAULT_DATABASE_CREDS_PATH"     default:"database/creds/dev-readonly"  description:"Temporary database credentials will be generated here"  long:"vault-database-creds-path"`

	// We will connect to this database using Vault-generated dynamic credentials
	DatabaseHostname string        ` env:"DATABASE_HOSTNAME"             required:"true"                        description:"PostgreSQL database hostname"                           long:"database-hostname"`
	DatabasePort     string        ` env:"DATABASE_PORT"                 default:"5432"                         description:"PostgreSQL database port"                               long:"database-port"`
	DatabaseName     string        ` env:"DATABASE_NAME"                 default:"postgres"                     description:"PostgreSQL database name"                               long:"database-name"`
	DatabaseTimeout  time.Duration ` env:"DATABASE_TIMEOUT"              default:"10s"                          description:"PostgreSQL database connection timeout"                 long:"database-timeout"`

	// A service which requires a specific secret API key (stored in Vault)
	SecureServiceAddress string `    env:"SECURE_SERVICE_ADDRESS"        required:"true"                        description:"3rd party service that requires secure credentials"     long:"secure-service-address"`
}
