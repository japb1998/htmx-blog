package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"

	"github.com/a-h/templ"
	"github.com/gorilla/sessions"
	"github.com/japb1998/htmx-blog/blog"
	"github.com/japb1998/htmx-blog/templates"
	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"
	"github.com/markbates/goth/providers/google"
)

var logHandler = slog.NewTextHandler(os.Stdout, nil).WithAttrs([]slog.Attr{slog.String("app", "blog")})
var logger = slog.New(logHandler)
var (
	ErrUserNotLoggedIn = echo.HTTPError{Message: "user not logged in", Code: http.StatusUnauthorized}
	ErrInvalidSession  = echo.HTTPError{Message: "invalid session", Code: http.StatusForbidden}
)

const sessionKey = "user_session"

func main() {

	err := godotenv.Load(".env")
	if err != nil {
		panic("error loading .env file")
	}
	// Fetch new store.
	store := sessions.NewFilesystemStore("./bin", []byte("secret-key"))

	if err != nil {
		panic(err)
	}
	store.MaxLength(8192)
	store.Options.HttpOnly = true
	store.Options.MaxAge = 3600 // 1 hour
	gothic.Store = store
	blogStore := blog.NewBlogStore()

	e := echo.New()
	e.Static("/static", "assets")

	goth.UseProviders(
		google.New(os.Getenv("GOOGLE_CLIENT_ID"), os.Getenv("GOOGLE_CLIENT_SECRET"), "http://localhost:8000/auth/google/callback", "email", "profile"),
	)

	e.GET("/", redirectMiddleware(func(ctx echo.Context) error {
		u := ctx.Get("user").(goth.User)

		component := templates.Home(u.Email)
		fmt.Println("component", component)
		if ctx.Request().Header.Get("Hx-Request") == "true" {
			return renderTempl(ctx, component)
		}

		indexWithHome := templates.Index(true, "Home", len(blogStore.GetAllBlogs()), component)

		return renderTempl(ctx, indexWithHome)
	}))
	e.GET("/login", func(ctx echo.Context) error {

		if session, err := store.Get(ctx.Request(), sessionKey); err != nil {
			return &ErrInvalidSession
		} else {
			_, ok := session.Values["user"]
			logger.Info("session value['user']", slog.Bool("ok", ok))
			if ok {
				return ctx.Redirect(http.StatusTemporaryRedirect, "/")
			}
		}

		/* Set header in order to not cache client side */
		ctx.Response().Header().Set(echo.HeaderCacheControl, "no-cache, no-store, must-revalidate")
		ctx.Response().Header().Set("Pragma", "no-cache")
		ctx.Response().Header().Set("Expires", "0")
		if ctx.Request().Header.Get("Hx-Request") == "true" {
			return renderTempl(ctx, templates.Login())
		}

		indexWithLogin := templates.Index(false, "Login", 0, templates.Login())

		return renderTempl(ctx, indexWithLogin)
	})

	e.GET("/editor", redirectMiddleware(func(c echo.Context) error {
		if c.Request().Header.Get("Hx-Request") == "true" {
			return renderTempl(c, templates.Editor())
		}

		indexWithEditor := templates.Index(true, "Editor", len(blogStore.GetAllBlogs()), templates.Editor())
		return renderTempl(c, indexWithEditor)
	}))

	e.GET("/count", sessionMiddleware(func(ctx echo.Context) error {

		c := len(blogStore.GetAllBlogs())

		return renderTempl(ctx, templates.PostCount(c))
	}))

	e.GET("/post", redirectMiddleware(func(ctx echo.Context) error {
		user := ctx.Get("user").(goth.User)
		posts := blogStore.GetAllBlogs()
		c := len(blogStore.GetAllBlogs())

		if ctx.Request().Header.Get("Hx-Request") == "true" {
			if ctx.QueryParam("justPosts") == "true" {
				return renderTempl(ctx, templates.PostList(user.Email, posts))
			}
			return renderTempl(ctx, templates.PostListPage(user.Email, posts))
		}

		indexWithPosts := templates.Index(true, "Posts", c, templates.PostListPage(user.Email, posts))
		return renderTempl(ctx, indexWithPosts)
	}))

	e.GET("/post/list", sessionMiddleware(func(ctx echo.Context) error {
		user := ctx.Get("user").(goth.User)
		posts := blogStore.GetAllBlogs()

		return renderTempl(ctx, templates.PostList(user.Email, posts))
	}))
	e.POST("/post", redirectMiddleware(func(ctx echo.Context) error {

		u := ctx.Get("user").(goth.User)
		logger.Info("user", slog.String("email", u.Email))
		var b blog.Blog
		if err := ctx.Bind(&b); err != nil {
			logger.Error("failed to bind request", slog.String("error", err.Error()))
			return fmt.Errorf("failed to bind request error=%w", err)
		}
		err := blogStore.AddBlog(u.Email, b)

		if err != nil {
			logger.Error("failed to add blog", slog.String("error", err.Error()))
			return fmt.Errorf("failed to add blog error=%w", err)
		}
		return renderTempl(ctx, templates.PostForm(""))
	}))

	e.DELETE("/post/:id", sessionMiddleware(func(ctx echo.Context) error {
		u := ctx.Get("user").(goth.User)

		id, err := strconv.Atoi(ctx.Param("id"))

		if err != nil {
			return ctx.JSON(http.StatusBadRequest, "invalid id")
		}

		blogStore.DeleteBlog(u.Email, id)

		return renderTempl(ctx, templates.PostList(u.Email, blogStore.GetAllBlogs()))
	}))

	auth := e.Group("/auth")

	{
		auth.GET("/:provider", func(c echo.Context) error {
			session, err := store.Get(c.Request(), "user_session")

			if err != nil {
				return err
			}

			if _, ok := session.Values["user"]; ok {
				/* avoids cache results from prev redirects */
				c.Response().Header().Set(echo.HeaderCacheControl, "no-cache, no-store, must-revalidate")
				c.Response().Header().Set("Pragma", "no-cache")
				c.Response().Header().Set("Expires", "0")
				return c.Redirect(http.StatusTemporaryRedirect, "/")
			}

			ctx := context.WithValue(c.Request().Context(), "provider", c.Param("provider"))

			gothic.BeginAuthHandler(c.Response(), c.Request().WithContext(ctx))

			return nil
		})

		auth.GET("/:provider/callback", func(c echo.Context) error {
			provider := c.Param("provider")
			ctx := context.WithValue(c.Request().Context(), "provider", provider)

			u, err := gothic.CompleteUserAuth(c.Response().Writer, c.Request().WithContext(ctx))
			if err != nil {
				logger.Error("error", slog.String("error", err.Error()))
				return err
			}

			// // gets a session
			session, err := store.Get(c.Request(), "user_session")

			if err != nil {
				return err
			}
			session.Values["user"] = u

			err = store.Save(c.Request(), c.Response().Writer, session)

			if err != nil {
				logger.Error("error", slog.String("error", err.Error()))
				return &ErrInvalidSession
			}

			/* Set header in order to not cache client side, used to avoid cached results from previous redirects.*/
			c.Response().Header().Set(echo.HeaderCacheControl, "no-cache, no-store, must-revalidate")
			c.Response().Header().Set("Pragma", "no-cache")
			c.Response().Header().Set("Expires", "0")
			return c.Redirect(http.StatusTemporaryRedirect, "/")
		})

		auth.GET("/logout", func(c echo.Context) error {
			session, err := store.Get(c.Request(), "user_session")

			if err != nil {
				return err
			}

			session.Options.MaxAge = -1
			store.Save(c.Request(), c.Response().Writer, session)

			return c.Redirect(http.StatusTemporaryRedirect, "/login")
		})

		auth.GET("/me", func(c echo.Context) error {
			session, err := store.Get(c.Request(), "user_session")

			if err != nil {
				return &ErrInvalidSession
			}

			u, ok := session.Values["user"]

			if !ok {
				return c.Redirect(http.StatusTemporaryRedirect, "/auth/google")
			}

			/* Set header in order to not cache client side */
			c.Response().Header().Set(echo.HeaderCacheControl, "no-cache, no-store, must-revalidate")
			c.Response().Header().Set("Pragma", "no-cache")
			c.Response().Header().Set("Expires", "0")
			return c.JSON(200, u)
		})
	}

	// views
	e.Logger.Fatal(e.Start(":8000"))
}

// renderTempl renders a templ component
func renderTempl(ctx echo.Context, cmp templ.Component) error {
	ctx.Response().Header().Set(echo.HeaderContentType, echo.MIMETextHTML)
	err := cmp.Render(ctx.Request().Context(), ctx.Response().Writer)
	if err != nil {
		logger.Error("failed to render component", slog.String("error", err.Error()))
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return nil
}

// sessionMiddleware validates the session and calls the function f if the session is valid
func sessionMiddleware(f func(echo.Context) error) func(echo.Context) error {

	return func(ctx echo.Context) error {
		session, err := gothic.Store.Get(ctx.Request(), sessionKey)

		if err != nil {
			logger.Error("failed to get session", slog.String("error", err.Error()))
			return &echo.HTTPError{Code: http.StatusInternalServerError, Message: ErrInvalidSession.Error}
		}

		if u, ok := session.Values["user"]; !ok {
			logger.Info("user is not logged in", slog.String("function", "sessionMiddleware"))
			return &ErrUserNotLoggedIn
		} else {
			ctx.Set("user", u)
			return f(ctx)
		}
	}
}

// sessionMiddleware with redirect
func redirectMiddleware(f func(echo.Context) error) func(echo.Context) error {

	return func(ctx echo.Context) error {
		session, err := gothic.Store.Get(ctx.Request(), sessionKey)

		if err != nil {
			logger.Error("failed to get session", slog.String("error", err.Error()))
			return ctx.Redirect(http.StatusTemporaryRedirect, "/login")
		}

		if u, ok := session.Values["user"]; !ok {
			logger.Info("user is not logged in", slog.String("function", "redirectMiddleware"))
			return ctx.Redirect(http.StatusTemporaryRedirect, "/login")
		} else {
			ctx.Set("user", u)
			return f(ctx)
		}
	}
}
