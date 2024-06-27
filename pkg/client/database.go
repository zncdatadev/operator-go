package client

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dbv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/database/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/util"
)

type DataBaseType string

const (
	Postgres DataBaseType = "postgresql"
	Mysql    DataBaseType = "mysql"
	Derby    DataBaseType = "derby"
	Unknown  DataBaseType = "unknown"
)

const (
	DbUsernameName = "USERNAME"
	DbPasswordName = "PASSWORD"
)

type DatabaseCredential struct {
	Username string `json:"USERNAME"`
	Password string `json:"PASSWORD"`
}

type DatabaseParams struct {
	DbType   DataBaseType
	Driver   string
	Username string
	Password string
	Host     string
	Port     int32
	DbName   string
}

func NewDatabaseParams(
	Driver string,
	Username string,
	Password string,
	Host string,
	Port int32,
	DbName string) *DatabaseParams {
	var dbType DataBaseType
	if strings.Contains(Driver, "postgresql") {
		dbType = Postgres
	}
	if strings.Contains(Driver, "mysql") {
		dbType = Mysql
	}
	if strings.Contains(Driver, "derby") {
		dbType = Derby
	}
	if Driver == "" {
		dbType = Unknown
	}
	return &DatabaseParams{
		DbType:   dbType,
		Driver:   Driver,
		Username: Username,
		Password: Password,
		Host:     Host,
		Port:     Port,
		DbName:   DbName,
	}
}

// DatabaseConfiguration is a struct that holds the configuration for a database.
// example1:
//
//	dbConfig := &DatabaseConfiguration{DbReference: &ref, Context: ctx, Client: client}
//	dbConfig.GetDatabaseParams()
//	dbConfig.GetURI()
//
// example2:
type DatabaseConfiguration struct {
	DbReference *string
	DbInline    *DatabaseParams
	Namespace   string
	Context     context.Context
	Client      *Client
}

func (d *DatabaseConfiguration) GetNamespace() string {
	if d.Namespace == "" {
		d.Namespace = d.Client.GetOwnerNamespace()
	}
	return d.Namespace
}

func (d *DatabaseConfiguration) GetRefDatabaseName() string {
	return *d.DbReference
}

func (d *DatabaseConfiguration) GetRefDatabase(ctx context.Context) (dbv1alpha1.Database, error) {
	databaseCR := &dbv1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: d.Client.GetOwnerNamespace(),
			Name:      d.GetRefDatabaseName(),
		},
	}
	if err := d.Client.Get(ctx, databaseCR); err != nil {
		return dbv1alpha1.Database{}, err
	}
	return *databaseCR, nil
}

func (d *DatabaseConfiguration) GetRefDatabaseConnection(name string) (dbv1alpha1.DatabaseConnection, error) {
	databaseConnectionCR := &dbv1alpha1.DatabaseConnection{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: d.GetNamespace(),
			Name:      name,
		},
	}

	if err := d.Client.Get(d.Context, databaseConnectionCR); err != nil {
		return dbv1alpha1.DatabaseConnection{}, err
	}
	return *databaseConnectionCR, nil
}

func (d *DatabaseConfiguration) GetCredential(name string) (*DatabaseCredential, error) {

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: d.GetNamespace(),
			Name:      name,
		},
	}

	if err := d.Client.Get(d.Context, secret); err != nil {
		return nil, err
	}

	username, err := util.Base64[[]byte]{Data: secret.Data[DbUsernameName]}.Decode()
	if err != nil {
		return nil, err
	}

	password, err := util.Base64[[]byte]{Data: secret.Data[DbPasswordName]}.Decode()
	if err != nil {
		return nil, err
	}

	return &DatabaseCredential{
		Username: string(username),
		Password: string(password),
	}, nil
}

func (d *DatabaseConfiguration) getDatabaseParamsFromResource() (*DatabaseParams, error) {
	db, err := d.GetRefDatabase(d.Context)
	if err != nil {
		return nil, err
	}
	credential := &DatabaseCredential{}

	if db.Spec.Credential.ExistSecret != "" {
		c, err := d.GetCredential(db.Spec.Credential.ExistSecret)
		if err != nil {
			return nil, err
		}
		credential = c
	} else {
		credential.Username = db.Spec.Credential.Username
		credential.Password = db.Spec.Credential.Password
	}

	dbConnection, err := d.GetRefDatabaseConnection(db.Spec.Reference)
	if err != nil {
		return nil, err
	}

	dbParams := &DatabaseParams{
		Username: credential.Username,
		Password: credential.Password,
	}

	provider := dbConnection.Spec.Provider

	if provider.Postgres != nil {
		dbParams.DbType = Postgres
		dbParams.Driver = provider.Postgres.Driver
		dbParams.Host = provider.Postgres.Host
		dbParams.Port = int32(provider.Mysql.Port)
		dbParams.DbName = db.Spec.DatabaseName
		return dbParams, nil
	} else if provider.Mysql != nil {
		dbParams.DbType = Mysql
		dbParams.Driver = provider.Mysql.Driver
		dbParams.Host = provider.Mysql.Host
		dbParams.Port = int32(provider.Mysql.Port)
		dbParams.DbName = db.Spec.DatabaseName
		return dbParams, nil
	} else {
		return &DatabaseParams{
			DbType:   Derby,
			Driver:   "",
			Username: "",
			Password: "",
			Host:     "",
			Port:     0,
			DbName:   "",
		}, nil
	}
}

func (d *DatabaseConfiguration) getDatabaseParamsFromInline() (*DatabaseParams, error) {
	return d.DbInline, nil
}

func (d *DatabaseConfiguration) GetDatabaseParams() (*DatabaseParams, error) {
	if d.DbReference != nil {
		return d.getDatabaseParamsFromResource()
	}
	if d.DbInline != nil {
		return d.getDatabaseParamsFromInline()
	}
	return nil, fmt.Errorf("invalid database configuration, dbReference and dbInline cannot be empty at the same time")
}

// GetJDBCUrl returns the JDBC URL for the database.
// Supported:
// - Postgres
// - Mysql
// - Derby
//   - `derby:dbName;create=true`, the dbName is a file path.
func (d *DatabaseConfiguration) GetJDBCUrl() (string, error) {
	if d.DbReference != nil {
		if refData, err := d.getDatabaseParamsFromResource(); err != nil {
			return "", err
		} else {
			return toJDBCUrl(*refData)
		}
	}
	if d.DbInline != nil {
		return toJDBCUrl(*d.DbInline)
	}
	return "", fmt.Errorf("invalid database configuration, dbReference and dbInline cannot be empty at the same time")
}

func toJDBCUrl(params DatabaseParams) (string, error) {
	var jdbcPrefix string
	switch params.DbType {
	case Mysql:
		jdbcPrefix = "jdbc:mysql"

		return fmt.Sprintf("%s://%s:%d/%s",
			jdbcPrefix,
			params.Host,
			params.Port,
			params.DbName,
		), nil

	case Postgres:
		jdbcPrefix = "jdbc:postgresql"
		return fmt.Sprintf("%s://%s:%d/%s",
			jdbcPrefix,
			params.Host,
			params.Port,
			params.DbName,
		), nil
	case Derby:
		jdbcPrefix = "jdbc:derby"
		return fmt.Sprintf("%s:%s;create=true",
			jdbcPrefix,
			params.DbName,
		), nil
	default:
		return "", fmt.Errorf("unknown jdbc prefix for driver %s", params.DbType)
	}
}
