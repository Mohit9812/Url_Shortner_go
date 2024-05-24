package routes

import (
	"URL_SHORTNER_GO/database"
	"URL_SHORTNER_GO/helpers"
	"os"
	"strconv"
	"time"
	"github.com/asaskevich/govalidator"
	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type request struct {
	URL						string 					`json:"url"`
	CustomShort		string 					`json:"short"`
	Expiry				time.Duration 	`json:"expiry"`
}

type response struct {
	URL								string				`json:"url"`
	CustomShort				string				`json:"short"`
	Expiry						time.Duration	`json:"expiry"`
	XRateRemaining		int						`json:"rate-limit"`
	XRateLimitReset		time.Duration	`json:"rate-limit-reset"`
}

func ShortenURL(c *fiber.Ctx) error {
	body := new(request)
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true, //"error" : "Invalid JSON"
			"message": "Invalid JSON",
		})
	}

	r2 := database.CreateClient(1)
	defer r2.Close()
	val, err := r2.Get(database.Ctx, c.IP()).Result()
	if err == redis.Nil {
		_ = r2.Set(database.Ctx, c.IP(), os.Getenv("API_QUOTA"), 30*60*time.Second)
	} else {
		val , _ := r2.Get(database.Ctx, c.IP()).Result()
		valInt, _ := strconv.Atoi(val)
		if valInt <= 0 {
			limit, _ := r2.TTL(database.Ctx, c.IP()).Result()
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": true, //"error" : "Rate limit exceeded"
				"message": "Rate limit exceeded",
				"rate_lmit_rest": limit/time.Nanosecond/time.Minute,
			})
		
		}
	}

	if !govalidator.IsURL(body.URL){
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": true, //"error" : "Invalid URL"
			"message": "Invalid URL",
		})
	}

	if !helpers.RemoveDomainError(body.URL){
		c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": true, //"error" : "URL is not accessible"
			"message": "URL is not accessible",
		})
	}

	body.URL = helpers.EnforceHTTP(body.URL)

	var id string

	if body.CustomShort == "" {
		id = uuid.New().String()[:6]
	} else {
		id = body.CustomShort
	}

	r := database.CreateClient(0)
	defer r.Close()

	val, _ = r.Get(database.Ctx, id).Result()
	if val != "" {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"error": true, //"error" : "Custom short URL already exists"
			"message": "Custom short URL already exists",
		})
	}

	if body.Expiry == 0 {
		body.Expiry = 24
	}

	err = r.Set(database.Ctx, id, body.URL, body.Expiry*3600*time.Second).Err()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": true, //"error" : "Internal Server Error"
			"message": "Internal Server Error",
		})
	}

	resp := response{
		URL:  						body.URL,
		CustomShort: 			"",
		Expiry:						body.Expiry,
		XRateRemaining:		10,
		XRateLimitReset:	30,
	}

	r2.Decr(database.Ctx, c.IP())

	val, _ = r2.Get(database.Ctx, c.IP()).Result()
	resp.XRateRemaining, _ = strconv.Atoi(val)

	ttl, _ := r2.TTL(database.Ctx, c.IP()).Result()

	resp.XRateLimitReset = ttl/time.Nanosecond/time.Minute

	resp.CustomShort = os.Getenv("DOMAIN") + "/" + id

	return c.Status(fiber.StatusCreated).JSON(resp)
}