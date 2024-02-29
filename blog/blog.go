package blog

import (
	"fmt"
	"sync"
	"time"
)

type Blog struct {
	ID        int       `form:"id,omitempty"`
	Title     string    `form:"title"`
	Body      string    `form:"body"`
	CreatedAt time.Time `form:"created_at,omitempty"`
	Creator   string    `form:"creator,omitempty"`
}

type BlogStore struct {
	mux   sync.RWMutex
	blogs map[string][]Blog
}

func NewBlogStore() *BlogStore {
	return &BlogStore{
		blogs: make(map[string][]Blog),
	}
}

func (bs *BlogStore) AddBlog(u string, b Blog) error {
	if u == "" {
		return fmt.Errorf("user cannot be empty")
	}

	bs.mux.Lock()
	defer bs.mux.Unlock()

	b.CreatedAt = time.Now()
	if len(bs.blogs[u]) == 0 {
		b.ID = 0
		b.Creator = u
	} else {
		b.ID = bs.blogs[u][len(bs.blogs[u])-1].ID + 1
		b.Creator = u
	}
	bs.blogs[u] = append(bs.blogs[u], b)
	return nil
}

func (bs *BlogStore) GetBlogs(u string) []Blog {
	bs.mux.RLock()
	defer bs.mux.RUnlock()
	return bs.blogs[u]
}

// GetAllBlogs returns all blogs from all users.
func (bs *BlogStore) GetAllBlogs() []Blog {
	bs.mux.RLock()
	defer bs.mux.RUnlock()
	var blogs []Blog
	for _, v := range bs.blogs {
		blogs = append(blogs, v...)
	}

	return blogs
}

func (bs *BlogStore) DeleteBlog(u string, id int) error {
	bs.mux.Lock()
	defer bs.mux.Unlock()

	for i, blog := range bs.blogs[u] {
		if blog.ID == id {
			bs.blogs[u] = append(bs.blogs[u][:i], bs.blogs[u][i+1:]...)
			return nil
		}
	}

	return fmt.Errorf("blog with id %d not found", id)
}
