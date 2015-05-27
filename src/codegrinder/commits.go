package main

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/go-martini/martini"
	"github.com/martini-contrib/render"
	"github.com/russross/meddler"
)

const (
	transcriptEventCountLimit = 500
	transcriptDataLimit       = 1e5
	openCommitTimeout         = 20 * time.Minute
)

type Commit struct {
	ID                int               `json:"id" meddler:"id,pk"`
	AssignmentID      int               `json:"assignmentID" meddler:"assignment_id"`
	ProblemStepNumber int               `json:"problemStepNumber" meddler:"problem_step_number"`
	UserID            int               `json:"userID" meddler:"user_id"`
	Action            string            `json:"action" meddler:"action,zeroisnull"`
	Closed            bool              `json:"closed" meddler:"closed"`
	Comment           string            `json:"comment" meddler:"comment,zeroisnull"`
	Score             float64           `json:"score" meddler:"score,zeroisnull"`
	ReportCard        *ReportCard       `json:"reportCard" meddler:"report_card,json"`
	Submission        map[string]string `json:"submission" meddler:"submission,json"`
	Transcript        []*EventMessage   `json:"transcript,omitempty" meddler:"transcript,json"`
	CreatedAt         time.Time         `json:"createdAt" meddler:"created_at,localtime"`
	UpdatedAt         time.Time         `json:"updatedAt" meddler:"updated_at,localtime"`
}

// GetUserMeAssignmentCommits handles requests to /api/v2/users/me/assignments/:assignment_id/commits,
// returning a list of commits for the given assignment for the current user.
func GetUserMeAssignmentCommits(w http.ResponseWriter, tx *sql.Tx, currentUser *User, params martini.Params, render render.Render) {
	assignmentID, err := strconv.Atoi(params["assignment_id"])
	if err != nil {
		loggedHTTPErrorf(w, http.StatusBadRequest, "error parsing assignment_id from URL: %v", err)
		return
	}

	commits := []*Commit{}
	if err := meddler.QueryAll(tx, &commits, `SELECT * FROM commits WHERE user_id = $1 AND assignment_id = $2 ORDER BY created_at`, currentUser.ID, assignmentID); err != nil {
		loggedHTTPErrorf(w, http.StatusInternalServerError, "db error getting commits for user %d and assignment %d: %v", currentUser.ID, assignmentID, err)
		return
	}
	render.JSON(http.StatusOK, commits)
}

// GetUserMeAssignmentCommitLast handles requests to /api/v2/users/me/assignments/:assignment_id/commits/last,
// returning the most recent commit for the given assignment for the current user.
func GetUserMeAssignmentCommitLast(w http.ResponseWriter, tx *sql.Tx, currentUser *User, params martini.Params, render render.Render) {
	assignmentID, err := strconv.Atoi(params["assignment_id"])
	if err != nil {
		loggedHTTPErrorf(w, http.StatusBadRequest, "error parsing assignment_id from URL: %v", err)
		return
	}

	commit := new(Commit)
	if err := meddler.QueryRow(tx, commit, `SELECT * FROM commits WHERE user_id = $1 AND assignment_id = $2 ORDER BY created_at DESC LIMIT 1`, currentUser.ID, assignmentID); err != nil {
		if err == sql.ErrNoRows {
			loggedHTTPErrorf(w, http.StatusNotFound, "no commit found for user %d and assignment %d", currentUser.ID, assignmentID)
		} else {
			loggedHTTPErrorf(w, http.StatusInternalServerError, "db error loading most recent commit for user %d and assignment %d: %v", currentUser.ID, assignmentID, err)
		}
		return
	}
	render.JSON(http.StatusOK, commit)
}

// GetUserMeAssignmentCommit handles requests to /api/v2/users/me/assignments/:assignment_id/commits/:commit_id,
// returning the given commit for the given assignment for the current user.
func GetUserMeAssignmentCommit(w http.ResponseWriter, tx *sql.Tx, currentUser *User, params martini.Params, render render.Render) {
	assignmentID, err := strconv.Atoi(params["assignment_id"])
	if err != nil {
		loggedHTTPErrorf(w, http.StatusBadRequest, "error parsing assignment_id from URL: %v", err)
		return
	}

	commitID, err := strconv.Atoi(params["commit_id"])
	if err != nil {
		loggedHTTPErrorf(w, http.StatusBadRequest, "error parsing commit_id from URL: %v", err)
		return
	}

	commit := new(Commit)
	if err := meddler.QueryRow(tx, commit, `SELECT * FROM commits WHERE id = $1 AND user_id = $2 AND assignment_id = $3`, commitID, currentUser.ID, assignmentID); err != nil {
		if err == sql.ErrNoRows {
			loggedHTTPErrorf(w, http.StatusNotFound, "no commit %d found for user %d and assignment %d", commitID, currentUser.ID, assignmentID)
		} else {
			loggedHTTPErrorf(w, http.StatusInternalServerError, "db error loading commit %d for user %d and assignment %d: %v", commitID, currentUser.ID, assignmentID, err)
		}
		return
	}
	render.JSON(http.StatusOK, commit)
}

// GetUserAssignmentCommits handles requests to /api/v2/users/:user_id/assignments/:assignment_id/commits,
// returning a list of commits for the given assignment for the given user.
func GetUserAssignmentCommits(w http.ResponseWriter, tx *sql.Tx, params martini.Params, render render.Render) {
	userID, err := strconv.Atoi(params["user_id"])
	if err != nil {
		loggedHTTPErrorf(w, http.StatusBadRequest, "error parsing assignment_id from URL: %v", err)
		return
	}

	assignmentID, err := strconv.Atoi(params["assignment_id"])
	if err != nil {
		loggedHTTPErrorf(w, http.StatusBadRequest, "error parsing assignment_id from URL: %v", err)
		return
	}

	commits := []*Commit{}
	if err := meddler.QueryAll(tx, &commits, `SELECT * FROM commits WHERE user_id = $1 AND assignment_id = $2 ORDER BY created_at`, userID, assignmentID); err != nil {
		loggedHTTPErrorf(w, http.StatusInternalServerError, "db error getting commits for user %d and assignment %d: %v", userID, assignmentID, err)
		return
	}
	render.JSON(http.StatusOK, commits)
}

// GetUserAssignmentCommitLast handles requests to /api/v2/users/:user_id/assignments/:assignment_id/commits/last,
// returning the most recent commit for the given assignment for the given user.
func GetUserAssignmentCommitLast(w http.ResponseWriter, tx *sql.Tx, params martini.Params, render render.Render) {
	userID, err := strconv.Atoi(params["user_id"])
	if err != nil {
		loggedHTTPErrorf(w, http.StatusBadRequest, "error parsing assignment_id from URL: %v", err)
		return
	}

	assignmentID, err := strconv.Atoi(params["assignment_id"])
	if err != nil {
		loggedHTTPErrorf(w, http.StatusBadRequest, "error parsing assignment_id from URL: %v", err)
		return
	}

	commit := new(Commit)
	if err := meddler.QueryRow(tx, commit, `SELECT * FROM commits WHERE user_id = $1 AND assignment_id = $2 ORDER BY created_at DESC LIMIT 1`, userID, assignmentID); err != nil {
		if err == sql.ErrNoRows {
			loggedHTTPErrorf(w, http.StatusNotFound, "no commit found for user %d and assignment %d", userID, assignmentID)
		} else {
			loggedHTTPErrorf(w, http.StatusInternalServerError, "db error loading most recent commit for user %d and assignment %d: %v", userID, assignmentID, err)
		}
		return
	}
	render.JSON(http.StatusOK, commit)
}

// GetUserAssignmentCommit handles requests to /api/v2/users/me/assignments/:assignment_id/commits/:commit_id,
// returning the given commit for the given assignment for the given user.
func GetUserAssignmentCommit(w http.ResponseWriter, tx *sql.Tx, params martini.Params, render render.Render) {
	userID, err := strconv.Atoi(params["user_id"])
	if err != nil {
		loggedHTTPErrorf(w, http.StatusBadRequest, "error parsing assignment_id from URL: %v", err)
		return
	}

	assignmentID, err := strconv.Atoi(params["assignment_id"])
	if err != nil {
		loggedHTTPErrorf(w, http.StatusBadRequest, "error parsing assignment_id from URL: %v", err)
		return
	}

	commitID, err := strconv.Atoi(params["commit_id"])
	if err != nil {
		loggedHTTPErrorf(w, http.StatusBadRequest, "error parsing commit_id from URL: %v", err)
		return
	}

	commit := new(Commit)
	if err := meddler.QueryRow(tx, commit, `SELECT * FROM commits WHERE id = $1 AND user_id = $2 AND assignment_id = $3`, commitID, userID, assignmentID); err != nil {
		if err == sql.ErrNoRows {
			loggedHTTPErrorf(w, http.StatusNotFound, "no commit %d found for user %d and assignment %d", commitID, userID, assignmentID)
		} else {
			loggedHTTPErrorf(w, http.StatusInternalServerError, "db error loading commit %d for user %d and assignment %d: %v", commitID, userID, assignmentID, err)
		}
		return
	}
	render.JSON(http.StatusOK, commit)
}

// DeleteUserAssignmentCommits handles requests to /api/v2/users/:user_id/assignments/:assignment_id/commits,
// deleting all commits for the given assignment for the given user.
func DeleteUserAssignmentCommits(w http.ResponseWriter, tx *sql.Tx, params martini.Params) {
	userID, err := strconv.Atoi(params["user_id"])
	if err != nil {
		loggedHTTPErrorf(w, http.StatusBadRequest, "error parsing assignment_id from URL: %v", err)
		return
	}

	assignmentID, err := strconv.Atoi(params["assignment_id"])
	if err != nil {
		loggedHTTPErrorf(w, http.StatusBadRequest, "error parsing assignment_id from URL: %v", err)
		return
	}

	if _, err := tx.Exec(`DELETE FROM commits WHERE user_id = $1 AND assignment_id = $2`, userID, assignmentID); err != nil {
		loggedHTTPErrorf(w, http.StatusInternalServerError, "db error deleting commits for assignment %d for user %d: %v", assignmentID, userID, err)
		return
	}
}

// DeleteUserAssignmentCommit handles requests to /api/v2/users/:user_id/assignments/:assignment_id/commits/:commit_id,
// deleting the given commits for the given assignment for the given user.
func DeleteUserAssignmentCommit(w http.ResponseWriter, tx *sql.Tx, params martini.Params) {
	userID, err := strconv.Atoi(params["user_id"])
	if err != nil {
		loggedHTTPErrorf(w, http.StatusBadRequest, "error parsing assignment_id from URL: %v", err)
		return
	}

	assignmentID, err := strconv.Atoi(params["assignment_id"])
	if err != nil {
		loggedHTTPErrorf(w, http.StatusBadRequest, "error parsing assignment_id from URL: %v", err)
		return
	}

	commitID, err := strconv.Atoi(params["commit_id"])
	if err != nil {
		loggedHTTPErrorf(w, http.StatusBadRequest, "error parsing commit_id from URL: %v", err)
		return
	}

	if _, err = tx.Exec(`DELETE FROM commits WHERE id = $1 AND user_id = $2 AND assignment_id = $3`, commitID, userID, assignmentID); err != nil {
		loggedHTTPErrorf(w, http.StatusInternalServerError, "db error deleting commit %d for assignment %d for user %d: %v", commitID, assignmentID, userID, err)
		return
	}
}

// PostUserAssignmentCommit handles requests to /api/v2/users/me/assignments/:assignment_id/commits,
// adding a new commit (or updating the most recent one) for the given assignment for the current user.
func PostUserAssignmentCommit(w http.ResponseWriter, tx *sql.Tx, currentUser *User, params martini.Params, commit Commit, render render.Render) {
	now := time.Now()

	assignmentID, err := strconv.Atoi(params["assignment_id"])
	if err != nil {
		loggedHTTPErrorf(w, http.StatusBadRequest, "error parsing assignment_id from URL: %v", err)
		return
	}

	assignment := new(Assignment)
	if err = meddler.QueryRow(tx, assignment, `SELECT * FROM assignments WHERE id = $1 AND user_id = $2`, assignmentID, currentUser.ID); err != nil {
		if err == sql.ErrNoRows {
			loggedHTTPErrorf(w, http.StatusNotFound, "no assignment %d found for user %d", assignmentID, currentUser.ID)
		} else {
			loggedHTTPErrorf(w, http.StatusInternalServerError, "db error loading assignment %d for user %d: %v", assignmentID, currentUser.ID, err)
		}
		return
	}

	// TODO: validate commit
	if len(commit.Submission) == 0 {
		loggedHTTPErrorf(w, http.StatusBadRequest, "commit does not contain any submission files")
		return
	}

	openCommit := new(Commit)
	if err = meddler.QueryRow(tx, openCommit, `SELECT * FROM commits WHERE NOT closed AND assignment_id = $1 LIMIT 1`, assignmentID); err != nil {
		if err == sql.ErrNoRows {
			openCommit = nil
		} else {
			loggedHTTPErrorf(w, http.StatusInternalServerError, "db error loading open commit for assignment %d for user %d: %v", assignmentID, currentUser.ID, err)
			return
		}
	}

	// close the old commit?
	if openCommit != nil && (now.Sub(openCommit.UpdatedAt) > openCommitTimeout || openCommit.ProblemStepNumber != commit.ProblemStepNumber) {
		openCommit.Closed = true
		openCommit.UpdatedAt = now
		if err := meddler.Update(tx, "commits", openCommit); err != nil {
			loggedHTTPErrorf(w, http.StatusInternalServerError, "db error closing old commit %d: %v", openCommit.ID, err)
			return
		}
		logi.Printf("closed old commit %d due to timeout/wrong step number", openCommit.ID)
		openCommit = nil
	}

	// update an existing commit?
	if openCommit != nil {
		commit.ID = openCommit.ID
		commit.CreatedAt = openCommit.CreatedAt
	} else {
		commit.ID = 0
		commit.CreatedAt = now
	}
	commit.AssignmentID = assignmentID
	commit.UserID = currentUser.ID
	if commit.ReportCard != nil || len(commit.Transcript) > 0 {
		commit.Closed = true
	}
	commit.UpdatedAt = now

	// TODO: sign the commit for execution

	if err := meddler.Save(tx, "commits", &commit); err != nil {
		loggedHTTPErrorf(w, http.StatusInternalServerError, "db error saving commit: %v", err)
		return
	}

	render.JSON(http.StatusOK, &commit)
}