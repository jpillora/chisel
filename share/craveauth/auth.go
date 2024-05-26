package craveauth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jpillora/chisel/share/cio"
	"gitlab.com/accupara/buildmeup/dcrpc"
	"golang.org/x/crypto/ssh"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

func postRequestWithClient(req *http.Request, timeout time.Duration, httpClient *http.Client) (body []byte, err error, statusCode int32) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if ctx != nil {
		req = req.WithContext(ctx)
	} else {
		err = errors.New("nil context")
		return nil, err, statusCode
	}

	var resp *http.Response
	resp, err = httpClient.Do(req)
	if err != nil {
		return nil, err, statusCode
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		body, err = ioutil.ReadAll(resp.Body)
		if strings.Contains(string(body), "Session does not exist") ||
			strings.Contains(string(body), "Access denied") {
			var v map[string]interface{}
			jsonerr := json.Unmarshal(body, &v)
			if jsonerr != nil {
				err = errors.New("Login expired")
			} else {
				err = fmt.Errorf("%s", v["msg"])
			}
		}
		statusCode = (int32)(resp.StatusCode)
		return
	default:
		body, _ = ioutil.ReadAll(resp.Body)
		err = errors.New(resp.Status)
		statusCode = (int32)(resp.StatusCode)
		return
	}
}

// PostRequest posts HTTP request using the default GoLang HTTP client
func PostRequest(req *http.Request, timeout time.Duration) (body []byte, err error, statusCode int32) {
	return postRequestWithClient(req, timeout, http.DefaultClient)
}

type GetUserResponseData struct {
	UserId int64 `json:"userId"`
}

type GetUserResponse struct {
	Success bool                `json:"success"`
	Data    GetUserResponseData `json:"data"`
}

func ValidateUser(password []byte, l *cio.Logger) (userId int64, err error) {
	return __validateUser(password, "crave-sshd", "ams", l)
}

func ValidateSignedInUser(password []byte, useragent string, host string, l *cio.Logger) (userId int64, err error) {
	return __validateUser(password, useragent, host, l)
}

func __validateUser(password []byte, useragent string, host string, l *cio.Logger) (userId int64, err error) {
	var url string
	var payload string
	var method string

	amsAPIUrl := os.Getenv("API_URL")
	if len(amsAPIUrl) == 0 {
		l.Infof("could not get API_URL")
		err = errors.New("Could not validate user")
		return
	}
	url = fmt.Sprintf("%s/ugrp/v1/getUser", amsAPIUrl)
	payload = fmt.Sprintf("{\"action\": \"id\"}")
	method = "POST"

	req, err := http.NewRequest(method, url, bytes.NewBuffer([]byte(payload)))
	if err != nil {
		l.Infof("could not create http req: %v", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", string(password))
	req.Header.Set("User-Agent", useragent)
	req.Header.Set("Referer", "sshd")
	req.Header.Set("Host", host)
	req.Host = host

	// res, _ := httputil.DumpRequest(req, true)

	body, err, _ := PostRequest(req, 24*time.Hour)
	if err != nil {
		l.Infof("could not validate user: %v", err)
		return
	}
	//l.Infof("response Body: %v", string(body))
	var resp GetUserResponse
	if err = json.Unmarshal(body, &resp); err != nil {
		l.Infof("Error parsing api response %v", err)
		return
	}

	// l.Infof("%v %v ", resp, resp.Data.UserId)
	if resp.Success {
		userId = resp.Data.UserId
	} else {
		l.Infof("Failed api response %v", resp)
		err = errors.New("Failed to authenticate user")
		return
	}

	return
}

func Auth(c ssh.ConnMetadata, password []byte, l *cio.Logger) (perms *ssh.Permissions, err error) {
	// check if user authentication is enabled and if not, allow all
	var p ssh.Permissions
	passwordString := string(password)

	if passwordString == "" {
		l.Infof("Unathenticated Access : Restricted to port 22\n")
		p.CriticalOptions = make(map[string]string)
		p.CriticalOptions["AllowedPorts"] = "22"
	} else {
		userId, err1 := ValidateUser(password, l)
		if err1 == nil {
			l.Infof("User accees granted to : %v", userId)
			p.CriticalOptions = make(map[string]string)
			p.CriticalOptions["AllowedUser"] = strconv.FormatInt(userId, 10)
		} else {
			l.Infof("User accees denied to %v", err1)
			err = err1
		}
	}
	l.Infof("err %v", err)
	return &p, err
}

type PortMap struct {
	Hostip      string `json:"hostip"`
	Hostport    int64  `json:"hostport"`
	Serviceport int64  `json:"serviceport"`
}

type ClientInfo struct {
	Host string    `json:"host"`
	Pm   []PortMap `json:"portmap"`
}

func CheckTargetUser(host string, tport string, userId string, l *cio.Logger) (jid int64, allowed bool, err error) {

	// select "ClientInfo" from build_jobinfo where "User_id" in (select "userId" from user_teams  where "teamId" in ( select "teamId" from user_teams where "userId"=33)) AND "Status" = 'running';
	// Don't worry about bobytabels since userId is generated from this code and not from user input
	query := "SELECT \"id\", \"ClientInfo\" FROM build_jobinfo " +
		"WHERE \"User_id\" IN (SELECT \"userId\" FROM user_teams " +
		"WHERE \"teamId\" IN ( SELECT \"teamId\" FROM user_teams " +
		"WHERE \"userId\"=" + userId + ")) " +
		"AND \"Status\" = 'running'"
	jid, allowed, err = GetAndCheckClientInfo(query,
		func(ci ClientInfo) (allowed bool) {
			tportint, _ := strconv.ParseInt(tport, 10, 64)
			if ci.Host == host {
				for _, v := range ci.Pm {
					if v.Hostport == tportint {
						l.Infof("Allowing user %v access to  : %v %v", userId, host, tportint)
						allowed = true
						break
					}
				}
			}
			return
		}, l)
	return
}

func CheckTargetConatinerPort(host string, tport string, cport string, l *cio.Logger) (allowed bool, err error) {
	query := "select \"id\", \"ClientInfo\" from build_jobinfo where \"ClientInfo\"::jsonb->>'host' = '" + host + "' AND \"Status\" = 'running'"
	_, allowed, err = GetAndCheckClientInfo(query,
		func(ci ClientInfo) (allowed bool) {
			tportint, _ := strconv.ParseInt(tport, 10, 64)
			cportint, _ := strconv.ParseInt(cport, 10, 64)
			if ci.Host == host {
				for _, v := range ci.Pm {
					if v.Hostport == tportint {
						if v.Serviceport == cportint {
							l.Infof("Allowing access to  : %v %v->%v", host, tportint, cportint)
							allowed = true
							break
						}
					}
				}
			}
			return
		}, l)
	return
}

func GetAndCheckClientInfo(query string, checkFunc func(ci ClientInfo) bool, l *cio.Logger) (jid int64, allowed bool, err error) {
	dbIP := os.Getenv("DB_HOST")
	if len(dbIP) == 0 {
		l.Infof("could not get DB_HOST")
		return
	}
	dbUser := os.Getenv("DB_USER")
	if len(dbUser) == 0 {
		l.Infof("could not get DB_USER")
		return
	}
	dbPass := os.Getenv("DB_PASS")
	if len(dbPass) == 0 {
		l.Infof("could not get DB_PASS")
		return
	}
	dbName := os.Getenv("DB_NAME")
	if len(dbName) == 0 {
		l.Infof("could not get DB_NAME")
		return
	}

	pgString := fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", dbUser, dbPass, dbIP, dbName)

	// urlExample := "postgres://username:password@localhost:5432/database_name"
	conn, err := pgx.Connect(context.Background(), pgString)
	if err != nil {
		l.Infof("Unable to connect to database: %v", err)
		return
	}
	defer conn.Close(context.Background())

	var ClientInfoString string

	rows, err := conn.Query(context.Background(), query)
	if err != nil {
		l.Infof("Query failed: %v", err)
		return
	}

	for rows.Next() {
		err = rows.Scan(&jid, &ClientInfoString)
		if err != nil {
			l.Infof("Row scanning failed: %v", err)
			return
		}
		defer rows.Close()

		var ci ClientInfo
		if err = json.Unmarshal([]byte(ClientInfoString), &ci); err != nil {
			l.Infof("Error scanning clientinfo json%v", err)
			return
		}

		allowed = checkFunc(ci)
		if allowed {
			break
		}
	}
	if err = rows.Err(); err != nil {
		l.Infof("rows error: %v", err)
		return
	}
	return
}

func ConnectDCMasterRPC(ip string, l *cio.Logger) (dcmasterClient dcrpc.DcMasterRPCClient, err error) {
	var conn *grpc.ClientConn
	hostUrl := fmt.Sprintf("%s:%v", ip, g.dcmasterPort)
	conn, err = grpc.Dial(hostUrl, grpc.WithInsecure(), grpc.WithBlock(), grpc.WithTimeout(time.Second*5))
	if nil != err {
		log.Printf("Failed to create RPC client for node %v. err = %v\n",
			hostUrl, err)
		return
	}
	defer conn.Close()

	dcmasterClient = dcrpc.NewDcMasterRPCClient(conn)
	return
}

func CheckForJob(proxyId string, dcmasterClient dcrpc.DcMasterRPCClient, jid int64) (err error) {
	downloadPatchObjectReq := &dcrpc.MasterStreamStdout{
		ProjectAndJob: &dcrpc.ProjectAndJob{
			ProjectId: g.pid,
			JobId:     g.jid,
		},
		IsStdError: false,
		Stdout:     fmt.Sprintf("Reverse proxy request for %v", proxproxyId),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Cancel ctx as soon as function returns.
	_, err = g.dcMasterClient.Trace(ctx, downloadPatchObjectReq)

	return
}
