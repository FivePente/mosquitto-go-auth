package backends

import (
	"testing"

	log "github.com/sirupsen/logrus"
	. "github.com/smartystreets/goconvey/convey"
)

func TestPostgres(t *testing.T) {

	//Initialize Postgres without mandatory values (fail).
	authOpts := make(map[string]string)
	authOpts["pg_host"] = "localhost"
	authOpts["pg_port"] = "5432"

	Convey("If mandatory params are not set initialization should fail", t, func() {
		_, err := NewPostgres(authOpts, log.DebugLevel)
		So(err, ShouldBeError)
	})

	//Initialize Postgres with some test values (omit tls).
	authOpts["pg_dbname"] = "go_auth_test"
	authOpts["pg_user"] = "go_auth_test"
	authOpts["pg_password"] = "go_auth_test"
	authOpts["pg_userquery"] = "SELECT password_hash FROM test_user WHERE username = $1 limit 1"
	authOpts["pg_superquery"] = "select count(*) from test_user where username = $1 and is_admin = true"
	authOpts["pg_aclquery"] = "SELECT test_acl.topic FROM test_acl, test_user WHERE test_user.username = $1 AND test_acl.test_user_id = test_user.id AND (rw = $2 or rw = 3)"

	Convey("Given valid params NewPostgres should return a Postgres backend instance", t, func() {
		postgres, err := NewPostgres(authOpts, log.DebugLevel)
		So(err, ShouldBeNil)

		//Empty db
		postgres.DB.MustExec("delete from test_user where 1 = 1")
		postgres.DB.MustExec("delete from test_acl where 1 = 1")

		//Insert a user to test auth
		username := "test"
		userPass := "testpw"
		//Hash generated by the pw utility
		userPassHash := "PBKDF2$sha512$100000$os24lcPr9cJt2QDVWssblQ==$BK1BQ2wbwU1zNxv3Ml3wLuu5//hPop3/LvaPYjjCwdBvnpwusnukJPpcXQzyyjOlZdieXTx6sXAcX4WnZRZZnw=="

		insertQuery := "INSERT INTO test_user(username, password_hash, is_admin) values($1, $2, $3) returning id"

		userID := 0

		iqErr := postgres.DB.Get(&userID, insertQuery, username, userPassHash, true)

		So(iqErr, ShouldBeNil)
		So(userID, ShouldBeGreaterThan, 0)

		Convey("Given a username and a correct password, it should correctly authenticate it", func() {

			authenticated := postgres.GetUser(username, userPass)
			So(authenticated, ShouldBeTrue)

		})

		Convey("Given a username and an incorrect password, it should not authenticate it", func() {

			authenticated := postgres.GetUser(username, "wrong_password")
			So(authenticated, ShouldBeFalse)

		})

		Convey("Given a username that is admin, super user should pass", func() {
			superuser := postgres.GetSuperuser(username)
			So(superuser, ShouldBeTrue)
		})

		//Now create some acls and test topics

		strictAcl := "test/topic/1"
		singleLevelAcl := "test/topic/+"
		hierarchyAcl := "test/#"

		userPattern := "test/%u"
		clientPattern := "test/%c"

		clientID := "test_client"

		aclID := 0
		aclQuery := "INSERT INTO test_acl(test_user_id, topic, rw) values($1, $2, $3) returning id"
		aqErr := postgres.DB.Get(&aclID, aclQuery, userID, strictAcl, 1)
		So(aqErr, ShouldBeNil)

		Convey("Given only strict acl in DB, an exact match should work and and inexact one not", func() {

			testTopic1 := `test/topic/1`
			testTopic2 := `test/topic/2`

			tt1 := postgres.CheckAcl(username, testTopic1, clientID, 1)
			tt2 := postgres.CheckAcl(username, testTopic2, clientID, 1)

			So(tt1, ShouldBeTrue)
			So(tt2, ShouldBeFalse)

		})

		Convey("Given read only privileges, a pub check should fail", func() {

			testTopic1 := "test/topic/1"
			tt1 := postgres.CheckAcl(username, testTopic1, clientID, 2)
			So(tt1, ShouldBeFalse)

		})

		Convey("Given wildcard subscriptions against strict db acl, acl checks should fail", func() {

			tt1 := postgres.CheckAcl(username, singleLevelAcl, clientID, 1)
			tt2 := postgres.CheckAcl(username, hierarchyAcl, clientID, 1)

			So(tt1, ShouldBeFalse)
			So(tt2, ShouldBeFalse)

		})

		//Now check against patterns.

		aqErr = postgres.DB.Get(&aclID, aclQuery, userID, userPattern, 1)
		So(aqErr, ShouldBeNil)

		Convey("Given a topic that mentions username, acl check should pass", func() {
			tt1 := postgres.CheckAcl(username, "test/test", clientID, 1)
			So(tt1, ShouldBeTrue)
		})

		aqErr = postgres.DB.Get(&aclID, aclQuery, userID, clientPattern, 1)
		So(aqErr, ShouldBeNil)

		Convey("Given a topic that mentions clientid, acl check should pass", func() {
			tt1 := postgres.CheckAcl(username, "test/test_client", clientID, 1)
			So(tt1, ShouldBeTrue)
		})

		//Now insert single level topic to check against.

		aqErr = postgres.DB.Get(&aclID, aclQuery, userID, singleLevelAcl, 1)
		So(aqErr, ShouldBeNil)

		Convey("Given a topic not strictly present that matches a db single level wildcard, acl check should pass", func() {
			tt1 := postgres.CheckAcl(username, "test/topic/whatever", clientID, 1)
			So(tt1, ShouldBeTrue)
		})

		//Now insert hierarchy wildcard to check against.

		aqErr = postgres.DB.Get(&aclID, aclQuery, userID, hierarchyAcl, 1)
		So(aqErr, ShouldBeNil)

		Convey("Given a topic not strictly present that matches a hierarchy wildcard, acl check should pass", func() {
			tt1 := postgres.CheckAcl(username, "test/what/ever", clientID, 1)
			So(tt1, ShouldBeTrue)
		})

		//Empty db
		postgres.DB.MustExec("delete from test_user where 1 = 1")
		postgres.DB.MustExec("delete from test_acl where 1 = 1")

		postgres.Halt()

	})

}
