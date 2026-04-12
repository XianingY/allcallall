package contact

import (
	"context"
	"errors"

	"github.com/allcallall/backend/internal/commerce"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/user"
)

// Service 联系人业务逻辑
// Service coordinates contact operations with user service.
type Service struct {
	repo       *Repository
	users      *user.Service
	commercial *commerce.Service
}

// NewService 构造函数
func NewService(repo *Repository, users *user.Service, commercial ...*commerce.Service) *Service {
	var commercialSvc *commerce.Service
	if len(commercial) > 0 {
		commercialSvc = commercial[0]
	}
	return &Service{
		repo:       repo,
		users:      users,
		commercial: commercialSvc,
	}
}

// ErrContactExists 联系人已存在
var ErrContactExists = errors.New("contact already exists")

// ErrSelfContact 不能添加自己为联系人
var ErrSelfContact = errors.New("cannot add yourself as contact")

// AddByEmail 通过邮箱添加联系人
func (s *Service) AddByEmail(ctx context.Context, ownerID uint64, ownerEmail, targetEmail string) error {
	target, err := s.users.GetByEmail(ctx, targetEmail)
	if err != nil {
		return err
	}
	if target.ID == ownerID {
		return ErrSelfContact
	}
	if s.commercial != nil {
		blocked, err := s.commercial.AreUsersBlocked(ctx, ownerID, target.ID)
		if err != nil {
			return err
		}
		if blocked {
			return commerce.ErrUserBlocked
		}
	}

	exists, err := s.repo.ContactExists(ctx, ownerID, target.ID)
	if err != nil {
		return err
	}
	if exists {
		return ErrContactExists
	}

	return s.repo.AddContact(ctx, ownerID, target.ID)
}

// Remove 删除联系人
func (s *Service) Remove(ctx context.Context, ownerID, contactID uint64) error {
	return s.repo.RemoveContact(ctx, ownerID, contactID)
}

// List 列出所有联系人
func (s *Service) List(ctx context.Context, ownerID uint64) ([]models.User, error) {
	return s.repo.ListContacts(ctx, ownerID)
}
