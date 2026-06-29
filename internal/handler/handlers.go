package handler

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/skip2/go-qrcode"

	"github.com/adham/hotel-qr-ordering/internal/model"
	"github.com/adham/hotel-qr-ordering/internal/repository"
	"github.com/adham/hotel-qr-ordering/internal/service"
)

type HTTPHandler struct {
	srv     *service.HotelService
	ws      *WSHub
	storage repository.StorageRepository
}

func NewHTTPHandler(s *service.HotelService, ws *WSHub, storage repository.StorageRepository) *HTTPHandler {
	return &HTTPHandler{
		srv:     s,
		ws:      ws,
		storage: storage,
	}
}

// --- Auth Routes ---

func (h *HTTPHandler) Signup(c *gin.Context) {
	var req model.AuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[ERROR] Signup bind JSON failed: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := h.srv.SignupAdmin(c.Request.Context(), req.Email, req.Password, req.PropertyName)
	if err != nil {
		log.Printf("[ERROR] Signup failed for email %s: %v", req.Email, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	token, err := h.srv.LoginAdmin(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		log.Printf("[ERROR] Auto-login failed after signup for email %s: %v", req.Email, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Signup succeeded but login failed"})
		return
	}

	// Set cookie
	c.SetCookie("admin_token", token, 86400, "/", "", false, true)
	log.Printf("[INFO] Admin signed up successfully: %s", req.Email)
	c.JSON(http.StatusCreated, gin.H{"message": "Signup successful", "user_id": user.ID, "token": token})
}

func (h *HTTPHandler) Login(c *gin.Context) {
	var req model.AuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[ERROR] Login bind JSON failed: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	token, err := h.srv.LoginAdmin(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		log.Printf("[ERROR] Login failed for email %s: %v", req.Email, err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	c.SetCookie("admin_token", token, 86400, "/", "", false, true)
	log.Printf("[INFO] Admin logged in successfully: %s", req.Email)
	c.JSON(http.StatusOK, gin.H{"message": "Login successful", "token": token})
}

// --- Admin Config Routes ---

func (h *HTTPHandler) ToggleService(c *gin.Context) {
	propID := c.GetString("property_id")
	var req model.ToggleServiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[ERROR] ToggleService bind JSON failed: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.srv.ToggleService(c.Request.Context(), propID, req.ServiceType, req.IsEnabled); err != nil {
		log.Printf("[ERROR] ToggleService failed for property %s, service %s: %v", propID, req.ServiceType, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[INFO] Service %s toggled to %t for property %s", req.ServiceType, req.IsEnabled, propID)
	c.JSON(http.StatusOK, gin.H{"message": "Service toggled successfully"})
}

func (h *HTTPHandler) UploadCatalogImage(c *gin.Context) {
	file, err := c.FormFile("image")
	if err != nil {
		log.Printf("[ERROR] UploadCatalogImage failed: form key 'image' missing")
		c.JSON(http.StatusBadRequest, gin.H{"error": "No image file uploaded (form key 'image' required)"})
		return
	}

	openedFile, err := file.Open()
	if err != nil {
		log.Printf("[ERROR] UploadCatalogImage failed to open file: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to open uploaded file: %v", err)})
		return
	}
	defer openedFile.Close()

	ext := filepath.Ext(file.Filename)
	uniqueFilename := fmt.Sprintf("%s%s", uuid.New().String(), ext)
	contentType := file.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	url, err := h.storage.UploadFile(c.Request.Context(), uniqueFilename, openedFile, contentType)
	if err != nil {
		log.Printf("[ERROR] UploadCatalogImage storage upload failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to upload image: %v", err)})
		return
	}

	log.Printf("[INFO] Image uploaded successfully: %s", url)
	c.JSON(http.StatusOK, gin.H{
		"url": url,
	})
}

// --- Client Routes ---

func (h *HTTPHandler) NegotiateSession(c *gin.Context) {
	var req model.NegotiateSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[ERROR] NegotiateSession bind JSON failed: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	token, err := h.srv.NegotiateGuestSession(c.Request.Context(), req.RoomStaticToken, req.OldGuestToken)
	if err != nil {
		log.Printf("[ERROR] NegotiateSession failed for room %s: %v", req.RoomStaticToken, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, model.NegotiateSessionResponse{
		SessionToken: token,
		Status:       "active",
	})
}

func (h *HTTPHandler) GetBootstrap(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		log.Printf("[ERROR] GetBootstrap failed: room token required")
		c.JSON(http.StatusBadRequest, gin.H{"error": "room token required"})
		return
	}

	res, _, err := h.srv.GetClientBootstrap(c.Request.Context(), token)
	if err != nil {
		log.Printf("[ERROR] GetClientBootstrap failed for token %s: %v", token, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, res)
}

func (h *HTTPHandler) GetClientOrders(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		log.Printf("[ERROR] GetClientOrders failed: room token required")
		c.JSON(http.StatusBadRequest, gin.H{"error": "room token required"})
		return
	}

	all := c.Query("all") == "true"

	orders, err := h.srv.GetClientOrders(c.Request.Context(), token, all)
	if err != nil {
		log.Printf("[ERROR] GetClientOrders failed for token %s: %v", token, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, orders)
}

func (h *HTTPHandler) CreateOrder(c *gin.Context) {
	var req model.OrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[ERROR] CreateOrder bind JSON failed: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	order, err := h.srv.PlaceOrder(c.Request.Context(), &req)
	if err != nil {
		log.Printf("[ERROR] PlaceOrder failed for room token %s: %v", req.RoomToken, err)
		if err.Error() == "session_expired_cannot_order" {
			c.JSON(http.StatusForbidden, gin.H{"error": "session_expired"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[INFO] Order placed successfully: ID %s, Room %s, Group ID %s", order.ID, order.RoomNumber, order.GroupID)
	c.JSON(http.StatusCreated, order)
}

// --- Admin Operational Routes ---

func (h *HTTPHandler) GetOrders(c *gin.Context) {
	propID := c.GetString("property_id")
	if propID == "" {
		// Fallback for testing if no JWT middleware applied
		propID = "11111111-1111-1111-1111-111111111111" // Grand Oasis
	}

	orders, err := h.srv.GetActiveOrders(c.Request.Context(), propID)
	if err != nil {
		log.Printf("[ERROR] GetActiveOrders failed for property %s: %v", propID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, orders)
}

func (h *HTTPHandler) UpdateOrderStatus(c *gin.Context) {
	orderID := c.Param("id")
	var req model.StatusUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[ERROR] UpdateOrderStatus bind JSON failed: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	order, err := h.srv.UpdateOrderStatus(c.Request.Context(), orderID, req.Status)
	if err != nil {
		log.Printf("[ERROR] UpdateOrderStatus failed for order %s to status %s: %v", orderID, req.Status, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[INFO] Order %s status updated to %s", orderID, req.Status)
	c.JSON(http.StatusOK, order)
}

func (h *HTTPHandler) RemoveOrderItem(c *gin.Context) {
	orderID := c.Param("id")
	itemID := c.Param("item_id")

	order, err := h.srv.RemoveItemFromOrder(c.Request.Context(), orderID, itemID)
	if err != nil {
		log.Printf("[ERROR] RemoveOrderItem failed for order %s, item %s: %v", orderID, itemID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[INFO] Item %s removed from order %s", itemID, orderID)
	c.JSON(http.StatusOK, order)
}

func (h *HTTPHandler) GetPropertyServices(c *gin.Context) {
	propID := c.GetString("property_id")
	services, err := h.srv.GetPropertyServices(c.Request.Context(), propID)
	if err != nil {
		log.Printf("[ERROR] GetPropertyServices failed for property %s: %v", propID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, services)
}

func (h *HTTPHandler) GetCatalogItems(c *gin.Context) {
	propID := c.GetString("property_id")
	search := c.Query("search")
	items, err := h.srv.GetCatalogItems(c.Request.Context(), propID, search)
	if err != nil {
		log.Printf("[ERROR] GetCatalogItems failed for property %s: %v", propID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *HTTPHandler) CreateCatalogItem(c *gin.Context) {
	propID := c.GetString("property_id")
	var item model.CatalogItem
	if err := c.ShouldBindJSON(&item); err != nil {
		log.Printf("[ERROR] CreateCatalogItem bind JSON failed: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	item.PropertyID = propID

	if err := h.srv.CreateCatalogItem(c.Request.Context(), &item); err != nil {
		log.Printf("[ERROR] CreateCatalogItem failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	log.Printf("[INFO] Catalog item created: ID %s, Name %s", item.ID, item.Name)
	c.JSON(http.StatusCreated, item)
}

func (h *HTTPHandler) UpdateCatalogItem(c *gin.Context) {
	propID := c.GetString("property_id")
	itemID := c.Param("id")

	var item model.CatalogItem
	if err := c.ShouldBindJSON(&item); err != nil {
		log.Printf("[ERROR] UpdateCatalogItem bind JSON failed: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	item.ID = itemID
	item.PropertyID = propID

	if err := h.srv.UpdateCatalogItem(c.Request.Context(), &item); err != nil {
		log.Printf("[ERROR] UpdateCatalogItem failed for item %s: %v", itemID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	log.Printf("[INFO] Catalog item updated: ID %s, Name %s", item.ID, item.Name)
	c.JSON(http.StatusOK, item)
}

func (h *HTTPHandler) DeleteCatalogItem(c *gin.Context) {
	propID := c.GetString("property_id")
	itemID := c.Param("id")

	if err := h.srv.DeleteCatalogItem(c.Request.Context(), itemID, propID); err != nil {
		log.Printf("[ERROR] DeleteCatalogItem failed for item %s: %v", itemID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	log.Printf("[INFO] Catalog item deleted: ID %s", itemID)
	c.JSON(http.StatusOK, gin.H{"message": "Item deleted successfully"})
}

func (h *HTTPHandler) CreateRoom(c *gin.Context) {
	propID := c.GetString("property_id")
	var req model.CreateRoomRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	room, err := h.srv.CreateRoom(c.Request.Context(), propID, req.RoomNumber, req.Floor, req.Building)
	if err != nil {
		log.Printf("[ERROR] CreateRoom failed for property %s: %v", propID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	log.Printf("[INFO] Room %s created for property %s", room.RoomNumber, propID)
	c.JSON(http.StatusCreated, room)
}

func (h *HTTPHandler) GetRooms(c *gin.Context) {
	propID := c.GetString("property_id")
	search := c.Query("search")
	rooms, err := h.srv.GetRooms(c.Request.Context(), propID, search)
	if err != nil {
		log.Printf("[ERROR] GetRooms failed for property %s: %v", propID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, rooms)
}

func (h *HTTPHandler) CheckoutRoom(c *gin.Context) {
	propID := c.GetString("property_id")
	roomID := c.Param("id")

	if err := h.srv.CheckoutRoom(c.Request.Context(), roomID, propID); err != nil {
		log.Printf("[ERROR] CheckoutRoom failed for room %s: %v", roomID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[INFO] Room %s checked out by property %s", roomID, propID)
	c.JSON(http.StatusOK, gin.H{"message": "Room checked out successfully"})
}

func (h *HTTPHandler) RotateRoomToken(c *gin.Context) {
	propID := c.GetString("property_id")
	roomID := c.Param("id")

	newToken, err := h.srv.RotateRoomToken(c.Request.Context(), roomID, propID)
	if err != nil {
		log.Printf("[ERROR] RotateRoomToken failed for room %s: %v", roomID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[INFO] Token rotated for room %s", roomID)
	c.JSON(http.StatusOK, gin.H{
		"message":  "Token rotated successfully",
		"qr_token": newToken,
	})
}

func (h *HTTPHandler) GetRoomQR(c *gin.Context) {
	propID := c.GetString("property_id")
	roomID := c.Param("id")

	rooms, err := h.srv.GetRooms(c.Request.Context(), propID, "")
	if err != nil {
		log.Printf("[ERROR] GetRoomQR GetRooms failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var targetRoom *model.Room
	for _, r := range rooms {
		if r.ID == roomID {
			targetRoom = &r
			break
		}
	}

	if targetRoom == nil {
		log.Printf("[ERROR] GetRoomQR failed: room %s not found", roomID)
		c.JSON(http.StatusNotFound, gin.H{"error": "room not found"})
		return
	}

	baseURL := os.Getenv("GUEST_PORTAL_BASE_URL")
	if baseURL == "" {
		baseURL = "https://devopsnawy.qzz.io"
	}

	qrURL := fmt.Sprintf("%s/order?token=%s", baseURL, targetRoom.QRToken)

	pngBytes, err := qrcode.Encode(qrURL, qrcode.Medium, 256)
	if err != nil {
		log.Printf("[ERROR] GetRoomQR generation failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to generate QR code: %v", err)})
		return
	}

	// Save to folder ./public/qrcodes/ for offline access and batch printing
	dir := "./public/qrcodes"
	_ = os.MkdirAll(dir, 0755)
	filePath := filepath.Join(dir, fmt.Sprintf("room_%s.png", targetRoom.RoomNumber))
	_ = os.WriteFile(filePath, pngBytes, 0644)

	c.Data(http.StatusOK, "image/png", pngBytes)
}
