package v1

import (
	"context"
	"fmt"
	"math"
	"strconv"

	v1 "github.com/GameComponent/economy-service/pkg/api/v1"
	"github.com/GameComponent/economy-service/pkg/helper/random"
	"github.com/google/uuid"
)

func (s *economyServiceServer) CreateStorage(ctx context.Context, req *v1.CreateStorageRequest) (*v1.CreateStorageResponse, error) {
	fmt.Println("CreateStorage")

	// check if the API version requested by client is supported by server
	if err := s.checkAPI(req.Api); err != nil {
		return nil, err
	}

	if req.GetName() == "" {
		return nil, fmt.Errorf("name should not be empty")
	}

	storage, err := s.storageRepository.Create(ctx, req.GetPlayerId(), req.GetName())
	if err != nil {
		fmt.Println("err")
		fmt.Println(err)
		return nil, fmt.Errorf("unable to create storage, make sure the player exists")
	}

	return &v1.CreateStorageResponse{
		Api:     apiVersion,
		Storage: storage,
	}, nil
}

func (s *economyServiceServer) GetStorage(ctx context.Context, req *v1.GetStorageRequest) (*v1.GetStorageResponse, error) {
	fmt.Println("GetStorage")

	// check if the API version requested by client is supported by server
	if err := s.checkAPI(req.Api); err != nil {
		return nil, err
	}

	// Check if the request
	if req.GetStorageId() == "" {
		return nil, fmt.Errorf("the request should contain the storage_id")
	}

	// Check if the requested storage id is a valid UUID
	_, err := uuid.Parse(req.GetStorageId())
	if err != nil {
		return nil, err
	}

	storage, err := s.storageRepository.Get(ctx, req.GetStorageId())
	if err != nil {
		return nil, err
	}

	return &v1.GetStorageResponse{
		Api:     apiVersion,
		Storage: storage,
	}, nil
}

func (s *economyServiceServer) ListStorage(ctx context.Context, req *v1.ListStorageRequest) (*v1.ListStorageResponse, error) {
	fmt.Println("ListStorage")

	// Parse the page token
	var parsedToken int64
	parsedToken, _ = strconv.ParseInt(req.GetPageToken(), 10, 32)

	// Get the limit
	limit := req.GetPageSize()
	if limit == 0 {
		limit = 100
	}

	// Get the offset
	offset := int32(0)
	if len(req.GetPageToken()) > 0 {
		offset = int32(parsedToken) * limit
	}

	// Get the players
	storages, totalSize, err := s.storageRepository.List(ctx, limit, offset)
	if err != nil {
		return nil, err
	}

	// Determine if there is a next page
	var nextPageToken string
	if totalSize > (offset + limit) {
		nextPage := int32(parsedToken) + 1
		nextPageToken = strconv.Itoa(int(nextPage))
	}

	return &v1.ListStorageResponse{
		Api:           apiVersion,
		Storages:      storages,
		NextPageToken: nextPageToken,
	}, nil
}

func (s *economyServiceServer) GiveCurrency(ctx context.Context, req *v1.GiveCurrencyRequest) (*v1.GiveCurrencyResponse, error) {
	fmt.Println("GiveCurrency")

	amount := random.GenerateRandomInt(
		req.GetAmount().MinAmount,
		req.GetAmount().MaxAmount,
	)

	// check if the API version requested by client is supported by server
	if err := s.checkAPI(req.Api); err != nil {
		return nil, err
	}

	storageCurrency, err := s.storageRepository.GiveCurrency(
		ctx,
		req.GetStorageId(),
		req.GetCurrencyId(),
		amount,
	)
	if err != nil {
		return nil, err
	}

	return &v1.GiveCurrencyResponse{
		Api:      apiVersion,
		Currency: storageCurrency,
	}, nil
}

func (s *economyServiceServer) GiveItem(ctx context.Context, req *v1.GiveItemRequest) (*v1.GiveItemResponse, error) {
	fmt.Println("GiveItem")

	// Check if the API version requested by client is supported by server
	if err := s.checkAPI(req.Api); err != nil {
		return nil, err
	}

	// Generate a random amount
	amount := random.GenerateRandomInt(
		req.GetAmount().MinAmount,
		req.GetAmount().MaxAmount,
	)

	// Create a remainder so whe know how many items still need to be created
	remainder := amount

	// Get the item
	item, err := s.itemRepository.Get(ctx, req.GetItemId())
	if err != nil {
		return nil, err
	}

	// Increase existing storage_items
	remainder, err = s.GiveToExistingStorageItems(
		ctx,
		req.GetStorageId(),
		req.GetItemId(),
		remainder,
		item,
	)
	if err != nil {
		return nil, err
	}

	// Create multiple unstackable items
	if item.Stackable == false && remainder > 0 {
		loops := int(remainder)
		for i := 0; i < loops; i++ {
			_, err := s.storageRepository.GiveItem(
				ctx,
				req.GetStorageId(),
				req.GetItemId(),
				1,
			)
			if err != nil {
				return nil, err
			}

			remainder--
		}
	}

	// For new stacks and full stack for unstackable items
	// and DEFAULT & UNBALANCED_CREATE_NEW_STACKS stack balancing methods
	if item.Stackable == true && remainder > 0 {
		resultAmounts := []int64{}

		if item.StackMaxCount == 0 {
			resultAmounts = append(resultAmounts, remainder)
		}

		if item.StackMaxCount > 0 {
			fullStacksToCreate := math.Floor(float64(remainder) / float64(item.StackMaxCount))

			for i := 0; i < int(fullStacksToCreate); i++ {
				resultAmounts = append(resultAmounts, item.StackMaxCount)
			}

			resultAmounts = append(resultAmounts, remainder%item.StackMaxCount)
		}

		for _, resultAmount := range resultAmounts {
			_, err := s.storageRepository.GiveItem(
				ctx,
				req.GetStorageId(),
				req.GetItemId(),
				resultAmount,
			)
			if err != nil {
				return nil, err
			}

			remainder -= resultAmount
		}
	}

	if remainder > 0 {
		return nil, fmt.Errorf("something went wrong")
	}

	return &v1.GiveItemResponse{
		Api:       apiVersion,
		StorageId: req.GetStorageId(),
		Amount:    amount,
	}, nil
}

func (s *economyServiceServer) GetExistingStorageItems(ctx context.Context, storageID string, itemID string) ([]*v1.StorageItem, error) {
	// Get the storage
	storage, err := s.storageRepository.Get(ctx, storageID)
	if err != nil {
		return nil, err
	}

	// Filter out storageItems that are not full
	existingStorageItems := []*v1.StorageItem{}
	for _, storageItem := range storage.Items {
		if storageItem.Item.Id != itemID {
			continue
		}

		if storageItem.Item.StackMaxCount > 0 && storageItem.Amount >= storageItem.Item.StackMaxCount {
			continue
		}

		existingStorageItems = append(existingStorageItems, storageItem)
	}

	return existingStorageItems, nil
}

func (s *economyServiceServer) GiveToExistingStorageItems(ctx context.Context, storageID string, itemID string, remainder int64, item *v1.Item) (int64, error) {
	if !item.Stackable {
		return remainder, nil
	}

	// Checks if item is stackable and new items should be added to existing stacks
	if item.StackBalancingMethod != v1.Item_BALANCED_FILL_EXISTING_STACKS && item.StackBalancingMethod != v1.Item_UNBALANCED_FILL_EXISTING_STACKS {
		return remainder, nil
	}

	// Get existing storageItems with the same item_id
	existingStorageItems, err := s.GetExistingStorageItems(
		ctx,
		storageID,
		itemID,
	)

	if err != nil {
		return remainder, err
	}

	if len(existingStorageItems) == 0 {
		return remainder, nil
	}

	// An existing stack already exists,
	// It does not have a max_amount so lets increase that one instead
	if item.StackMaxCount == 0 {
		storageItem := existingStorageItems[0]

		err := s.storageRepository.IncreaseItemAmount(
			ctx,
			storageItem.Id,
			remainder,
		)
		if err != nil {
			return remainder, err
		}

		remainder = 0
	}

	// Because there is a stack_max_amount we should not accidentally overflow it
	// So we'll first try to spread if over the existing stacks
	if item.StackMaxCount > 0 {
		for _, existingStorageItem := range existingStorageItems {
			// Calculate the remaining space
			existingStorageItemRemainder := item.StackMaxCount - existingStorageItem.Amount

			// Calculate the amount to increase
			existingStorageItemIncrease := remainder
			if remainder >= existingStorageItemRemainder {
				existingStorageItemIncrease = existingStorageItemRemainder
			}

			err := s.storageRepository.IncreaseItemAmount(
				ctx,
				existingStorageItem.Id,
				existingStorageItemIncrease,
			)
			if err != nil {
				return remainder, err
			}

			remainder -= existingStorageItemIncrease
		}
	}

	return remainder, nil
}
