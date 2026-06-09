package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/adham/hotel-qr-ordering/internal/model"
	"github.com/adham/hotel-qr-ordering/internal/service"
)

type HTTPHandler struct {
	srv *service.HotelService
	ws  *WSHub
}

func NewHTTPHandler(s *service.HotelService, ws *WSHub) *HTTPHandler {
	return &HTTPHandler{
		srv: s,
		ws:  ws,
	}
}

// --- Auth Routes ---

func (h *HTTPHandler) Signup(c *gin.Context) {
	var req model.AuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := h.srv.SignupAdmin(c.Request.Context(), req.Email, req.Password, req.PropertyName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	token, err := h.srv.LoginAdmin(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Signup succeeded but login failed"})
		return
	}

	// Set cookie
	c.SetCookie("admin_token", token, 86400, "/", "", false, true)

	c.JSON(http.StatusCreated, gin.H{"message": "Signup successful", "user_id": user.ID, "token": token})
}

func (h *HTTPHandler) Login(c *gin.Context) {
	var req model.AuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	token, err := h.srv.LoginAdmin(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	c.SetCookie("admin_token", token, 86400, "/", "", false, true)

	c.JSON(http.StatusOK, gin.H{"message": "Login successful", "token": token})
}

// --- Admin Config Routes ---

func (h *HTTPHandler) ToggleService(c *gin.Context) {
	propID := c.GetString("property_id")
	var req model.ToggleServiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.srv.ToggleService(c.Request.Context(), propID, req.ServiceType, req.IsEnabled); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Service toggled successfully"})
}

func (h *HTTPHandler) UploadCatalogImage(c *gin.Context) {
	// Simulate CDN upload
	c.JSON(http.StatusOK, gin.H{
		"url": "https://images.unsplash.com/photo-1546069901-ba9599a7e63c?auto=format&fit=crop&w=500&q=60",
	})
}

// --- Client Routes ---

func (h *HTTPHandler) GetBootstrap(c *gin.Context) {
	room := c.Query("room")
	if room == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "room number required"})
		return
	}

	res, _, err := h.srv.GetClientBootstrap(c.Request.Context(), room)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, res)
}

func (h *HTTPHandler) GetClientOrders(c *gin.Context) {
	room := c.Query("room")
	if room == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "room number required"})
		return
	}

	orders, err := h.srv.GetClientOrders(c.Request.Context(), room)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, orders)
}

func (h *HTTPHandler) CreateOrder(c *gin.Context) {
	var req model.OrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	order, err := h.srv.PlaceOrder(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, orders)
}

func (h *HTTPHandler) UpdateOrderStatus(c *gin.Context) {
	orderID := c.Param("id")
	var req model.StatusUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	order, err := h.srv.UpdateOrderStatus(c.Request.Context(), orderID, req.Status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, order)
}

func (h *HTTPHandler) RemoveOrderItem(c *gin.Context) {
	orderID := c.Param("id")
	itemID := c.Param("item_id")

	order, err := h.srv.RemoveItemFromOrder(c.Request.Context(), orderID, itemID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, order)
}

func (h *HTTPHandler) GetPropertyServices(c *gin.Context) {
	propID := c.GetString("property_id")
	services, err := h.srv.GetPropertyServices(c.Request.Context(), propID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, services)
}

func (h *HTTPHandler) GetCatalogItems(c *gin.Context) {
	propID := c.GetString("property_id")
	items, err := h.srv.GetCatalogItems(c.Request.Context(), propID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *HTTPHandler) CreateCatalogItem(c *gin.Context) {
	propID := c.GetString("property_id")
	var item model.CatalogItem
	if err := c.ShouldBindJSON(&item); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	item.PropertyID = propID

	if err := h.srv.CreateCatalogItem(c.Request.Context(), &item); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, item)
}

func (h *HTTPHandler) UpdateCatalogItem(c *gin.Context) {
	propID := c.GetString("property_id")
	itemID := c.Param("id")

	var item model.CatalogItem
	if err := c.ShouldBindJSON(&item); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	item.ID = itemID
	item.PropertyID = propID

	if err := h.srv.UpdateCatalogItem(c.Request.Context(), &item); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *HTTPHandler) DeleteCatalogItem(c *gin.Context) {
	propID := c.GetString("property_id")
	itemID := c.Param("id")

	if err := h.srv.DeleteCatalogItem(c.Request.Context(), itemID, propID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Item deleted successfully"})
}
