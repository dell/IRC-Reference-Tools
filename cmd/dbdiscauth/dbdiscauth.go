package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dell/IRC-Reference-Tools/internal/logging"
	"github.com/dell/IRC-Reference-Tools/internal/loginauth"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/pkg/auth"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/pkg/databus"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/pkg/messagebus"
	"github.com/dell/iDRAC-Telemetry-Reference-Tools/pkg/wire"
	_ "github.com/go-sql-driver/mysql"
)

var configStrings = map[string]string{
	//default settings
	"mysqluser":     "",             //to be provided by user
	"mysqlpwd":      "",             //to be provieed by user
	"mysqlHost":     "mysqldb",      //to be provided by user
	"mysqlHostPort": "3306",         //to be provided by user
	"mysqlDBName":   "irc_reftools", //to be provided by user
}

func GetAllAssociatedSystemsFromDB(db *sql.DB, ircIP string) ([]auth.ServiceItem, error) {
	rows, err := db.Query("SELECT ip FROM AssociatedSystem WHERE ircIp = ?", ircIP)
	if err != nil {
		return nil, fmt.Errorf("failed to get associated systems from DB: %v", err)
	}
	defer rows.Close()

	var systems []auth.ServiceItem
	for rows.Next() {
		var item auth.ServiceItem
		if err := rows.Scan(&item.Ip); err != nil {
			return nil, fmt.Errorf("failed to scan row: %v", err)
		}
		systems = append(systems, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %v", err)
	}

	return systems, nil
}

func GetIRCFromDB(db *sql.DB) ([]auth.Service, error) {
	results, err := db.Query("SELECT serviceType, ip, authType, auth, state FROM IRC")
	if err != nil {
		return nil, err
	}
	var ret []auth.Service
	for results.Next() {
		var value auth.Service
		var tmp string
		err = results.Scan(&value.ServiceType, &value.Ip, &value.AuthType, &tmp, &value.State)
		if err != nil {
			return nil, fmt.Errorf("error scanning results %v", err)
		}
		err := json.Unmarshal([]byte(tmp), &value.Auth)
		if err != nil {
			return nil, fmt.Errorf("error unmarshal auth %v", err)
		}
		ret = append(ret, value)
	}
	return ret, nil
}

func AddIRCToDB(db *sql.DB, service auth.Service, authService *auth.AuthorizationService) error {
	stmt, err := db.Prepare("INSERT INTO IRC(serviceType, ip, authType, auth) VALUES(?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close() // Close statement to prevent leak
	jsonStr, err := json.Marshal(service.Auth)
	if err != nil {
		return err
	}
	_, err = stmt.Exec(service.ServiceType, service.Ip, service.AuthType, string(jsonStr))
	if err != nil {
		return err
	}
	err = AddValveStateToDB(db, service.Ip)
	if err == nil {
		logging.Info("IP added to ValveStatus Table")
	}
	_ = authService.BroadcastService(service)
	return nil
}

func AddValveStateToDB(db *sql.DB, ip string) error {
	stmt, err := db.Prepare("INSERT INTO ValveStatus(ircip) VALUES(?)")
	if err != nil {
		return err
	}
	defer stmt.Close() // Close statement to prevent leak
	_, err = stmt.Exec(ip)
	if err != nil {
		return err
	}
	return nil
}

func hashPasswordSHA256Hex(password string) string {
	sum := sha256.Sum256([]byte(password))
	return hex.EncodeToString(sum[:])
}

func UpdateLoginToDB(db *sql.DB, username string, password string, jwtVersion int) error {
	if username == "" {
		return fmt.Errorf("username cannot be empty")
	}
	passHash := hashPasswordSHA256Hex(password)
	stmt, err := db.Prepare(`INSERT INTO Login (username, pass_hash, jwt_version)
		VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE pass_hash = VALUES(pass_hash), jwt_version = VALUES(jwt_version), updated_at = CURRENT_TIMESTAMP`)
	if err != nil {
		return err
	}
	defer stmt.Close() // Close statement to prevent leak
	_, err = stmt.Exec(username, passHash, jwtVersion)
	return err
}

func AddSystemTypeToDB(db *sql.DB, systemType auth.SystemType) error {
	stmt, err := db.Prepare("INSERT INTO SystemType(description) VALUES(?)")
	if err != nil {
		return err
	}
	defer stmt.Close() // Close statement to prevent leak
	_, err = stmt.Exec(systemType)
	if err != nil {
		return err
	}
	return nil
}
func AddAssociatedSystemToDB(db *sql.DB, serviceitems auth.ServiceItem, authService *auth.AuthorizationService) error {
	stmt, err := db.Prepare(`INSERT INTO AssociatedSystem(ip, ircIp, systemtype, authType, auth, state, isForcefulShutdown, timeout)
                              VALUES(?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close() // Close statement to prevent leak

	jsonStr, err := json.Marshal(serviceitems.Auth)
	if err != nil {
		return err
	}

	_, err = stmt.Exec(serviceitems.Ip, serviceitems.ServiceIP, serviceitems.Systemtypedesc, serviceitems.AuthType, string(jsonStr), serviceitems.State, serviceitems.IsForcefulshutdown, serviceitems.Timeout)
	if err != nil {
		return err
	}
	return authService.BroadcastService(serviceitems.Service)
}

func UpdateIRCToDB(db *sql.DB, service auth.Service, authService *auth.AuthorizationService) error {
	stmt, err := db.Prepare("UPDATE IRC SET state = ? WHERE ip = ?")
	if err != nil {
		return err
	}
	defer stmt.Close() // Close statement to prevent leak
	_, err = stmt.Exec(service.State, service.Ip)
	if err != nil {
		return err
	}
	return nil
}

func getAssociatedSystemsFromDB(db *sql.DB, ircIP string) ([]auth.ServiceItem, error) {
	logging.Info("Associated systems query", "ircIP", ircIP)
	query := `SELECT
	a.ip,
	a.ircIp,
	a.serviceType,           -- numeric type
	a.systemtype,
	a.authType,
	a.auth,
	a.state,
	a.isForcefulShutdown,
	a.timeout
	FROM AssociatedSystem AS a
	LEFT JOIN SystemType AS st
	ON st.description = a.systemtype
	WHERE a.ircIp = ?;`

	results, err := db.Query(query, ircIP)
	if err != nil {
		return nil, fmt.Errorf("query error: %w", err)
	}
	defer results.Close()

	var ret []auth.ServiceItem
	for results.Next() {
		var value auth.ServiceItem
		var tmp string
		var SystemType string
		err = results.Scan(&value.Ip, &value.ServiceIP, &value.ServiceType, &SystemType, &value.AuthType, &tmp, &value.State, &value.IsForcefulshutdown, &value.Timeout)
		if err != nil {
			return nil, fmt.Errorf("error scanning results: %w", err)
		}

		if err := json.Unmarshal([]byte(tmp), &value.Auth); err != nil {
			return nil, fmt.Errorf("error unmarshalling auth: %w", err)
		}

		value.Systemtypedesc = SystemType
		ret = append(ret, value)
	}
	logging.Info("Returning associated systems", "count", len(ret), "systems", ret)
	return ret, nil
}

func getAssociatedSystemFromDB(db *sql.DB, systemIP string) (*auth.ServiceItem, error) {
	query := `SELECT
	a.ip,
	a.ircIp,
	a.serviceType,           -- numeric type
	a.systemtype,
	a.authType,
	a.auth,
	a.state,
	a.isForcefulShutdown,
	a.timeout
	FROM AssociatedSystem AS a
	LEFT JOIN SystemType AS st
	ON st.description = a.systemtype
	WHERE a.ip = ?
	LIMIT 1;`

	row := db.QueryRow(query, systemIP)
	var value auth.ServiceItem
	var tmp string
	var SystemType string
	err := row.Scan(&value.Ip, &value.ServiceIP, &value.ServiceType, &SystemType, &value.AuthType, &tmp, &value.State, &value.IsForcefulshutdown, &value.Timeout)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal([]byte(tmp), &value.Auth); err != nil {
		return nil, fmt.Errorf("error unmarshalling auth: %w", err)
	}

	value.Systemtypedesc = SystemType
	return &value, nil
}

func GetValveStateFromDB(db *sql.DB) ([]auth.ValveState, error) {
	results, err := db.Query("SELECT ircip, vstate1, vstate2 FROM ValveStatus")
	if err != nil {
		return nil, err
	}
	var ret []auth.ValveState
	for results.Next() {
		var value auth.ValveState
		err = results.Scan(&value.Ip, &value.VState1, &value.VState2)
		if err != nil {
			return nil, err
		}
		ret = append(ret, value)
	}
	return ret, nil
}

func GetSystemTypeFromDB(db *sql.DB) ([]auth.SystemType, error) {
	results, err := db.Query("SELECT description FROM SystemType")
	if err != nil {
		return nil, err
	}
	var ret []auth.SystemType
	for results.Next() {
		var value auth.SystemType
		err = results.Scan(&value)
		if err != nil {
			return nil, err
		}
		ret = append(ret, value)
	}
	return ret, nil
}

func GetLoginFromDB(db *sql.DB) (auth.Login, error) {
	results, err := db.Query("SELECT username, pass_hash, jwt_version FROM Login ORDER BY updated_at DESC LIMIT 1")
	if err != nil {
		return auth.Login{}, err
	}
	var value auth.Login
	for results.Next() {
		err = results.Scan(&value.Username, &value.Password, &value.JwtVersion)
		if err != nil {
			return auth.Login{}, err
		}
	}
	return value, nil
}

func UpdateAssosiatedSystemstoDB(db *sql.DB, serviceitems auth.ServiceItem, authService *auth.AuthorizationService, queue string) error {
	stmt, err := db.Prepare(`UPDATE AssociatedSystem SET state = ? WHERE ip = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close() // Close statement to prevent leak
	_, err = stmt.Exec(serviceitems.State, serviceitems.Ip)
	if err != nil {
		return err
	}
	return nil

}

func UpdateValveState1ToDB(db *sql.DB, ValveStatus auth.ValveState) error {
	stmt, err := db.Prepare("UPDATE ValveStatus SET vstate1 = ? WHERE ircip = ?")
	if err != nil {
		return err
	}
	defer stmt.Close() // Close statement to prevent leak

	_, err = stmt.Exec(ValveStatus.VState1, ValveStatus.Ip)
	if err != nil {
		return err
	}
	return nil
}

func UpdateValveState2ToDB(db *sql.DB, ValveStatus auth.ValveState) error {
	stmt, err := db.Prepare("UPDATE ValveStatus SET vstate2 = ? WHERE ircip = ?")
	if err != nil {
		return err
	}
	defer stmt.Close() // Close statement to prevent leak

	_, err = stmt.Exec(ValveStatus.VState2, ValveStatus.Ip)
	if err != nil {
		return err
	}
	return nil
}

func UpdateServiceXToDB(db *sql.DB, state string, ip string) error {
	logging.Info("UpdateServiceXToDB", "state", state, "ip", ip)
	stmt, err := db.Prepare("UPDATE IRC SET state = ? WHERE ip = ?")
	if err != nil {
		return err
	}
	defer stmt.Close() // Close statement to prevent leak

	_, err = stmt.Exec(state, ip)
	if err != nil {
		return err
	}
	stmt2, err := db.Prepare("UPDATE AssociatedSystem SET state = ? WHERE ip = ?")
	if err != nil {
		return err
	}
	defer stmt2.Close() // Close statement to prevent leak

	_, err = stmt2.Exec(state, ip)
	if err != nil {
		return err
	}
	return nil
}
func DeleteIRCFromDB(db *sql.DB, service auth.Service) error {
	stmt, err := db.Prepare("DELETE FROM IRC WHERE ip = ?")
	if err != nil {
		return err
	}
	defer stmt.Close() // Close statement to prevent leak
	_, err = stmt.Exec(service.Ip)
	if err != nil {
		return err
	}
	return nil
}

func DeleteAssociatedSystemFromDB(db *sql.DB, serviceitem auth.ServiceItem) error {
	stmt, err := db.Prepare("DELETE FROM AssociatedSystem WHERE ip = ?")
	if err != nil {
		return err
	}
	defer stmt.Close() // Close statement to prevent leak
	_, err = stmt.Exec(serviceitem.Ip)
	if err != nil {
		return err
	}
	return nil
}

func SeedDefaultAdmin(db *sql.DB) error {
	// Use the same hashing as the rest of the code
	defaultHash := hashPasswordSHA256Hex(loginauth.DefaultAdmin)

	// Insert only if missing (no overwrite)
	// ON DUPLICATE KEY UPDATE username=username is a no-op
	_, err := db.Exec(`
        INSERT INTO Login (username, pass_hash, jwt_version)
        VALUES (?, ?, 1)
        ON DUPLICATE KEY UPDATE username = username;`,
		loginauth.DefaultAdmin, defaultHash,
	)
	return err
}

func initMySQLDatabase() (*sql.DB, error) {
	var db *sql.DB
	var err error

	// Connect to the postgresql database
	connStr := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s",
		configStrings["mysqluser"],
		configStrings["mysqlpwd"],
		configStrings["mysqlHost"],
		configStrings["mysqlHostPort"],
		configStrings["mysqlDBName"])

	// Debug log: dont print out sensitive info.
	// DONT LOG PASSWORDS! - instead we log "X"s the same length as the password.
	logging.Info("Database connection string",
		"user", configStrings["mysqluser"],
		"host", configStrings["mysqlHost"],
		"port", configStrings["mysqlHostPort"],
		"database", configStrings["mysqlDBName"])

	// Add timeout for database connection
	timeout := time.After(2 * time.Minute)

ConnectionLoop:
	for {
		select {
		case <-timeout:
			return nil, fmt.Errorf("database connection timeout after 2 minutes")
		default:
			db, err = sql.Open("mysql", connStr)
			if err != nil {
				logging.Error("Could not connect to mysql database", "err", err)
				time.Sleep(5 * time.Second)
				continue
			}

			// Test the connection
			if err := db.Ping(); err != nil {
				logging.Error("Database ping failed", "err", err)
				time.Sleep(5 * time.Second)
				continue
			}

			break ConnectionLoop
		}
	}

	// Create tables with timeout
	tableTimeout := time.After(1 * time.Minute)

TableLoop:
	for {
		select {
		case <-tableTimeout:
			return nil, fmt.Errorf("table creation timeout after 1 minute")
		default:
			_, err := db.Query(`CREATE TABLE IF NOT EXISTS IRC(
		serviceType INT, 
		ip VARCHAR(255) PRIMARY KEY, 
		authType INT, auth VARCHAR(4096), 
		state VARCHAR(255) DEFAULT '')
		ENGINE=InnoDB ENCRYPTION='Y';`)
			if err != nil {
				logging.Error("Could not create DB Table", "err", err)
				time.Sleep(5 * time.Second)
				continue
			}
			break TableLoop
		}
	}

	systemTypeTimeout := time.After(30 * time.Second)
SystemTypeLoop:
	for {
		select {
		case <-systemTypeTimeout:
			return nil, fmt.Errorf("SystemType table creation timeout")
		default:
			_, err := db.Exec(`CREATE TABLE IF NOT EXISTS SystemType (
		description VARCHAR(255) PRIMARY KEY NOT NULL)
		ENGINE=InnoDB ENCRYPTION='Y';`)
			if err != nil {
				logging.Error("Could not create SystemType table", "err", err)
				time.Sleep(5 * time.Second)
				continue
			}
			break SystemTypeLoop
		}
	}

	associatedSystemTimeout := time.After(30 * time.Second)
AssociatedSystemLoop:
	for {
		select {
		case <-associatedSystemTimeout:
			return nil, fmt.Errorf("AssociatedSystem table creation timeout")
		default:
			_, err := db.Query(`CREATE TABLE IF NOT EXISTS AssociatedSystem (
		ip VARCHAR(255) PRIMARY KEY NOT NULL,       -- System's own IP
		ircIp VARCHAR(255) NOT NULL,          -- Foreign key referencing IRC(ip)
		serviceType INT DEFAULT 0,
		systemtype VARCHAR(255) NOT NULL,
		authType INT,
		auth VARCHAR(4096),
		state VARCHAR(255) DEFAULT '',
		isForcefulShutdown TINYINT(1) DEFAULT true,
		timeout INT DEFAULT 0,
		FOREIGN KEY (ircIp) REFERENCES IRC(ip) ON DELETE CASCADE,
		FOREIGN KEY (systemtype) REFERENCES SystemType(description))
		ENGINE=InnoDB ENCRYPTION='Y';`)
			if err != nil {
				logging.Error("Could not create AssociatedSystem table", "err", err)
				time.Sleep(5 * time.Second)
				continue
			}
			break AssociatedSystemLoop
		}
	}

	valveStatusTimeout := time.After(30 * time.Second)
ValveStatusLoop:
	for {
		select {
		case <-valveStatusTimeout:
			return nil, fmt.Errorf("ValveStatus table creation timeout")
		default:
			_, err := db.Exec(`CREATE TABLE IF NOT EXISTS ValveStatus (
		ircip VARCHAR(255) PRIMARY KEY NOT NULL,
		vstate1 VARCHAR(255) NOT NULL DEFAULT 'Unknown',
		vstate2 VARCHAR(255) NOT NULL DEFAULT 'Unknown',
		FOREIGN KEY (ircIp) REFERENCES IRC(ip) ON DELETE CASCADE)
		ENGINE=InnoDB ENCRYPTION='Y';`)
			if err != nil {
				logging.Error("Could not create AuthType table", "err", err)
				time.Sleep(5 * time.Second)
				continue
			}
			break ValveStatusLoop
		}
	}

	loginTimeout := time.After(30 * time.Second)
LoginLoop:
	for {
		select {
		case <-loginTimeout:
			return nil, fmt.Errorf("Login table creation timeout")
		default:
			_, err := db.Exec(`CREATE TABLE IF NOT EXISTS Login (
		username VARCHAR(64) PRIMARY KEY NOT NULL,
		pass_hash VARCHAR(255) NOT NULL,
		jwt_version INT NOT NULL DEFAULT 1,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
		) ENGINE=InnoDB ENCRYPTION='Y';`)
			if err != nil {
				logging.Error("Could not create login table", "err", err)
				time.Sleep(5 * time.Second)
				continue
			}
			break LoginLoop
		}
	}

	seedTimeout := time.After(30 * time.Second)
SeedLoop:
	for {
		select {
		case <-seedTimeout:
			return nil, fmt.Errorf("default admin seeding timeout")
		default:
			if err := SeedDefaultAdmin(db); err != nil {
				logging.Error("Could not insert default admin login", "err", err)
				time.Sleep(5 * time.Second)
				continue
			}
			break SeedLoop
		}
	}

	logging.Info("Database tables and trigger creation completed")
	return db, nil
}

func getEnvSettings() {
	username := os.Getenv("MYSQL_USER")
	if len(username) > 0 {
		configStrings["mysqluser"] = username
	}
	pwd := os.Getenv("MYSQL_PASSWORD")
	if len(pwd) > 0 {
		configStrings["mysqlpwd"] = pwd
	}
	host := os.Getenv("MYSQL_HOST")
	if len(host) > 0 {
		configStrings["mysqlHost"] = host
	}
	hostport := os.Getenv("MYSQL_HOST_PORT")
	if len(host) > 0 {
		configStrings["mysqlHostPort"] = hostport
	}
	dbname := os.Getenv("MYSQL_DATABASE")
	if len(dbname) > 0 {
		configStrings["mysqlDBName"] = dbname
	}
	logging.Info("Environment settings loaded")
}

func main() {
	getEnvSettings()
	mb := messagebus.InitializeStompMB()
	defer mb.Close()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	authorizationService := auth.NewAuthServiceWithBus(mb)

	//Initialize mysql db instance which stores service authorizations
	db, err := initMySQLDatabase()
	if err != nil {
		logging.Error("Failed to initialize db", "err", err)
		logging.Error("Critical services unavailable, exiting")
		os.Exit(1)
	}
	defer db.Close()
	logging.Info("Main loop started, waiting for commands")
	envelopes := make(chan wire.Envelope)
	DataBusClient := databus.NewDataBusClientWithBus(mb, "dbdiscauth")
	go authorizationService.ReceiveEnvelopes(envelopes) //nolint: errcheck

	for {
		select {
		case env, ok := <-envelopes:
			if !ok {
				logging.Error("Envelopes channel closed, shutting down")
				return
			}
			logging.Info("Received command in dbdiscauth", "command", env.Type)
			switch env.Type {
			case auth.GETSERVICE:
				var svc auth.Service
				if err := wire.DecodePayload(env.Payload, &svc); err != nil {
					logging.Error("Failed to decode service payload", "err", err)
					authorizationService.ReplyError(env, err) //nolint: errcheck
					break
				}
				ircIP := svc.Ip
				logging.Info("GETSERVICE request", "ircIP", ircIP)
				services, err := GetIRCFromDB(db)
				if err != nil {
					logging.Error("Failed to get db entries", "err", err)
					authorizationService.ReplyError(env, err) //nolint: errcheck
					break
				}
				found := false
				for _, element := range services {
					if element.Ip == ircIP {
						logging.Debug("GETSERVICE: service found", "ip", ircIP, "service", element)
						authorizationService.Reply(env, element) //nolint: errcheck
						found = true
						break
					}
				}
				if !found {
					authorizationService.ReplyError(env, fmt.Errorf("service not found for IP: %s", ircIP)) //nolint: errcheck
				}
			case auth.RESEND:
				authServices, err := GetIRCFromDB(db)
				logging.Info("RESEND: republishing all IRC services and their associated systems")
				if err != nil {
					logging.Error("Failed to get db entries", "err", err)
					break
				}
				for _, element := range authServices {
					logging.Info("Broadcasting auth service", "service", element)
					go authorizationService.BroadcastService(element) //nolint: errcheck
					// Also resend all associated systems (service items) for this IRC.
					systems, err := getAssociatedSystemsFromDB(db, element.Ip)
					if err != nil {
						logging.Error("Failed to get associated systems", "ip", element.Ip, "err", err)
						break
					}
					for _, si := range systems {
						// redfishread listens for auth.Service events. ServiceItems embed a Service,
						// so we resend the embedded Service.
						go authorizationService.BroadcastService(si.Service) //nolint: errcheck
					}
				}
			case auth.ADDSERVICE:
				var svc auth.Service
				if err := wire.DecodePayload(env.Payload, &svc); err != nil {
					logging.Error("Failed to decode service payload", "err", err)
					break
				}
				err = AddIRCToDB(db, svc, authorizationService)
				if err != nil {
					logging.Error("Addservice,Failed to write db entries", "err", err)
				}
			case auth.ADDSERVICEITEM:
				var si auth.ServiceItem
				if err := wire.DecodePayload(env.Payload, &si); err != nil {
					logging.Error("Failed to decode service item payload", "err", err)
					break
				}
				err = AddAssociatedSystemToDB(db, si, authorizationService)
				logging.Info("ADDSERVICEITEM request received")
				if err != nil {
					logging.Error("Addserviceitem,Failed to write db entries", "err", err)
				}
			case auth.GETALLSERVICES:
				services, err := GetIRCFromDB(db)
				logging.Info("GETALLSERVICES: fetched services", "services", services)
				if err != nil {
					logging.Error("Failed to get db entries", "err", err)
					// Send empty list on error so caller doesn't block
					authorizationService.ReplyError(env, fmt.Errorf("failed to get db entries: %v", err)) //nolint: errcheck
					break
				}
				// Send the services back to configui
				authorizationService.Reply(env, services) //nolint: errcheck
			case auth.GETSERVICEITEMS:
				var si auth.ServiceItem
				if err := wire.DecodePayload(env.Payload, &si); err != nil {
					logging.Error("Failed to decode service item payload", "err", err)
					authorizationService.ReplyError(env, err) //nolint: errcheck
					break
				}
				ircIP := si.ServiceIP
				systems, err := getAssociatedSystemsFromDB(db, ircIP)
				if err != nil {
					logging.Error("Failed to get db entries", "err", err)
					// Send empty list on error so caller doesn't block
					authorizationService.Reply(env, []auth.ServiceItem{}) //nolint: errcheck
					break
				}
				authorizationService.Reply(env, systems) //nolint: errcheck
			case auth.GETSERVICEITEM:
				var si auth.ServiceItem
				if err := wire.DecodePayload(env.Payload, &si); err != nil {
					logging.Error("Failed to decode service item payload", "err", err)
					authorizationService.ReplyError(env, err) //nolint: errcheck
					break
				}
				logging.Debug("GETSERVICEITEM received", "serviceItem", si)
				systemIP := si.Service.Ip
				if systemIP == "" {
					logging.Warn("GETSERVICEITEM missing service item IP")
					authorizationService.ReplyError(env, fmt.Errorf("missing service item IP")) //nolint: errcheck
					break
				}
				siResult, err := getAssociatedSystemFromDB(db, systemIP)
				if err != nil {
					logging.Error("Failed to get service item from db", "ip", systemIP, "err", err)
					authorizationService.ReplyError(env, fmt.Errorf("failed to get service item %s from db: %v", systemIP, err)) //nolint: errcheck
					break
				}
				logging.Info("Associated system found", "serviceItem", siResult)
				authorizationService.Reply(env, []auth.ServiceItem{*siResult}) //nolint: errcheck
			case auth.UPDATESERVICE:
				var svc auth.Service
				if err := wire.DecodePayload(env.Payload, &svc); err != nil {
					logging.Error("Failed to decode service payload", "err", err)
					break
				}
				logging.Info("UPDATESERVICE request", "state", svc.State)
				err = UpdateIRCToDB(db, svc, authorizationService)
				if err != nil {
					logging.Error("Updateservice,Failed to write db entries", "err", err)
				}
			case auth.UPDATESERVICEITEM:
				var si auth.ServiceItem
				if err := wire.DecodePayload(env.Payload, &si); err != nil {
					logging.Error("Failed to decode service item payload", "err", err)
					break
				}
				logging.Info("UPDATESERVICEITEM request", "state", si.State)
				err = UpdateAssosiatedSystemstoDB(db, si, authorizationService, "")
				if err != nil {
					logging.Error("Updateserviceitem,Failed to write db entries", "err", err)
				}
			case auth.UPDATESERVICEX:
				var svc auth.Service
				if err := wire.DecodePayload(env.Payload, &svc); err != nil {
					logging.Error("Failed to decode service payload", "err", err)
					break
				}
				logging.Info("UPDATESERVICEX request", "state", svc.State)
				err = UpdateServiceXToDB(db, svc.State, svc.Ip)
				if err != nil {
					logging.Error("UpdateserviceX,Failed to write db entries", "err", err)
				}
			case auth.ADDSYSTEMTYPE:
				var sysType auth.SystemType
				if err := wire.DecodePayload(env.Payload, &sysType); err != nil {
					logging.Error("Failed to decode system type payload", "err", err)
					break
				}
				logging.Info("ADDSYSTEMTYPE request", "systemType", sysType)
				err = AddSystemTypeToDB(db, sysType)
				if err != nil {
					logging.Error("AddSystemType,Failed to write db entries", "err", err)
				}
			case auth.UPDATEVALVESTATE:
				var vs auth.ValveState
				if err := wire.DecodePayload(env.Payload, &vs); err != nil {
					logging.Error("Failed to decode valve state payload", "err", err)
					break
				}
				logging.Info("UPDATEVALVESTATE request", "valveState", vs)
				if vs.VState1 != "" {
					err = UpdateValveState1ToDB(db, vs)
					if err != nil {
						logging.Error("UpdateValveState (vstate1) failed to write db entries", "err", err)
					}
				}
				if vs.VState2 != "" {
					err = UpdateValveState2ToDB(db, vs)
					if err != nil {
						logging.Error("UpdateValveState (vstate2) failed to write db entries", "err", err)
					}
				}
			case auth.GETVALVESTATE:
				valvestate, err := GetValveStateFromDB(db)
				if err != nil {
					logging.Error("Failed to get db entries", "err", err)
					// Send empty list on error so caller doesn't block
					authorizationService.ReplyError(env, err) //nolint: errcheck
					break
				}
				authorizationService.Reply(env, valvestate) //nolint: errcheck
			case auth.GETSYSTEMTYPES:
				systemtypes, err := GetSystemTypeFromDB(db)
				if err != nil {
					logging.Error("Failed to get db entries", "err", err)
					// Send empty list on error so caller doesn't block
					authorizationService.ReplyError(env, err) //nolint: errcheck
					break
				}
				authorizationService.Reply(env, systemtypes) //nolint: errcheck
			case auth.DELETESERVICE:
				var svc auth.Service
				if err := wire.DecodePayload(env.Payload, &svc); err != nil {
					logging.Error("Failed to decode service payload", "err", err)
					continue
				}
				systemlists, err := getAssociatedSystemsFromDB(db, svc.Ip)
				if err != nil {
					logging.Error("Deleteservice,Failed to get associated systems from db", "err", err)
				}
				for _, system := range systemlists {
					DataBusClient.DeleteProducer(system.Service)
				}
				err = DeleteIRCFromDB(db, svc)
				if err != nil {
					logging.Error("Deleteservice,Failed to delete db entries", "err", err)
				} else {
					logging.Info("Deleted from database", "service", svc)
					DataBusClient.DeleteProducer(svc)
				}
			case auth.DELETESERVICEITEM:
				var si auth.ServiceItem
				if err := wire.DecodePayload(env.Payload, &si); err != nil {
					logging.Error("Failed to decode service item payload", "err", err)
					continue
				}
				err := DeleteAssociatedSystemFromDB(db, si)
				if err != nil {
					logging.Error("Deleteserviceitem,Failed to delete db entries", "err", err)
				} else {
					DataBusClient.DeleteProducer(si.Service)
					time.Sleep(2 * time.Second)
				}
			case auth.UPDATELOGIN:
				var l auth.Login
				if err := wire.DecodePayload(env.Payload, &l); err != nil {
					logging.Error("Failed to decode login payload", "err", err)
					continue
				}
				err := UpdateLoginToDB(db, l.Username, l.Password, l.JwtVersion)
				if err != nil {
					logging.Error("UpdateLogin,Failed to update db entries", "err", err)
				}
			case auth.GETLOGIN:
				login, err := GetLoginFromDB(db)
				if err != nil {
					logging.Error("GetLogin,Failed to get db entries", "err", err)
					authorizationService.ReplyError(env, err)
				} else {
					authorizationService.Reply(env, login)
				}
			case auth.TERMINATE:
				os.Exit(0)
			}
		case <-sigChan:
			logging.Info("Received signal, shutting down")
			return
		}
	}
}
