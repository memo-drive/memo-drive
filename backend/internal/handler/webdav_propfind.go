package handler

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/service"
)

type webDAVMultiStatus struct {
	XMLName   xml.Name            `xml:"D:multistatus"`
	Namespace string              `xml:"xmlns:D,attr"`
	Responses []webDAVXMLResponse `xml:"D:response"`
}

type webDAVXMLResponse struct {
	Href      string                `xml:"D:href"`
	PropStats []webDAVXMLPropStatus `xml:"D:propstat"`
}

type webDAVXMLPropStatus struct {
	Prop   webDAVXMLPropSet `xml:"D:prop"`
	Status string           `xml:"D:status"`
}

type webDAVXMLPropSet struct {
	Properties []webDAVXMLProperty
}

type webDAVXMLProperty struct {
	Name       string
	Value      string
	Collection bool
}

type webDAVPropfindMode string

const (
	webDAVPropfindAllProp  webDAVPropfindMode = "allprop"
	webDAVPropfindPropName webDAVPropfindMode = "propname"
	webDAVPropfindProp     webDAVPropfindMode = "prop"
)

type webDAVPropfindRequest struct {
	Mode  webDAVPropfindMode
	Props []string
}

func handleWebDAVPropfind(c *fiber.Ctx, webdav *service.WebDAVService, resource *service.WebDAVResource) error {
	started := time.Now()
	if resource == nil {
		log.Printf("level=warn component=webdav event=propfind_rejected method=%s virtual_path=%q depth=%q status=%d reason=%q",
			c.Method(), cleanWebDAVLogPath(webDAVVirtualPathLocalValue(c)), strings.TrimSpace(c.Get("Depth")), fiber.StatusNotFound, "resource_not_found")
		return c.SendStatus(fiber.StatusNotFound)
	}
	propfind, err := parseWebDAVPropfind(c.Body())
	if err != nil {
		log.Printf("level=warn component=webdav event=propfind_rejected method=%s virtual_path=%q depth=%q status=%d reason=%q body_bytes=%d err=%q",
			c.Method(), cleanWebDAVLogPath(webDAVVirtualPathLocalValue(c)), strings.TrimSpace(c.Get("Depth")), fiber.StatusBadRequest, "parse_error", len(c.Body()), err)
		return c.SendStatus(fiber.StatusBadRequest)
	}
	depth := strings.TrimSpace(c.Get("Depth"))
	if depth == "" {
		depth = "0"
	}
	if strings.EqualFold(depth, "infinity") {
		log.Printf("level=warn component=webdav event=propfind_rejected method=%s virtual_path=%q depth=%q mode=%s props=%d status=%d reason=%q",
			c.Method(), cleanWebDAVLogPath(resource.VirtualPath), depth, propfind.Mode, len(propfind.Props), fiber.StatusForbidden, "depth_infinity")
		return c.SendStatus(fiber.StatusForbidden)
	}
	resources := []*service.WebDAVResource{resource}
	if depth == "1" && resource.IsDir() {
		children, err := webdav.ListChildren(c.Context(), resource.VirtualPath)
		if err != nil {
			log.Printf("level=error component=webdav event=propfind_failed method=%s virtual_path=%q depth=%q mode=%s props=%d err=%q",
				c.Method(), cleanWebDAVLogPath(resource.VirtualPath), depth, propfind.Mode, len(propfind.Props), err)
			return err
		}
		for i := range children {
			child := children[i]
			resources = append(resources, &service.WebDAVResource{
				VirtualPath: path.Join(child.Path, child.Name),
				File:        &child,
			})
		}
	}
	var usage *service.StorageUsage
	var quotaErr error
	if webDAVPropfindNeedsQuota(propfind) {
		usage, quotaErr = webdav.StorageUsage(c.Context())
	}
	responses := make([]webDAVXMLResponse, 0, len(resources))
	for _, item := range resources {
		responses = append(responses, webDAVResponseForResource(item, propfind, usage, quotaErr))
	}
	body, err := xml.Marshal(webDAVMultiStatus{
		Namespace: "DAV:",
		Responses: responses,
	})
	if err != nil {
		log.Printf("level=error component=webdav event=propfind_failed method=%s virtual_path=%q depth=%q mode=%s props=%d resources=%d err=%q",
			c.Method(), cleanWebDAVLogPath(resource.VirtualPath), depth, propfind.Mode, len(propfind.Props), len(responses), err)
		return err
	}
	c.Set(fiber.HeaderContentType, "application/xml; charset=utf-8")
	log.Printf("level=info component=webdav event=propfind_complete method=%s virtual_path=%q depth=%q mode=%s props=%d resources=%d status=%d duration_ms=%d",
		c.Method(), cleanWebDAVLogPath(resource.VirtualPath), depth, propfind.Mode, len(propfind.Props), len(responses), fiber.StatusMultiStatus, time.Since(started).Milliseconds())
	return c.Status(fiber.StatusMultiStatus).Send(body)
}

func webDAVResponseForResource(resource *service.WebDAVResource, propfind webDAVPropfindRequest, usage *service.StorageUsage, quotaErr error) webDAVXMLResponse {
	return webDAVXMLResponse{
		Href:      webDAVHref(resource.VirtualPath, resource.IsDir()),
		PropStats: webDAVPropStatsFor(resource, propfind, usage, quotaErr),
	}
}

func (p webDAVXMLPropSet) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	start.Name.Local = "D:prop"
	if err := e.EncodeToken(start); err != nil {
		return err
	}
	for _, property := range p.Properties {
		if err := e.Encode(property); err != nil {
			return err
		}
	}
	return e.EncodeToken(start.End())
}

func (p webDAVXMLProperty) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	start.Name.Local = "D:" + p.Name
	if err := e.EncodeToken(start); err != nil {
		return err
	}
	if p.Collection {
		collection := xml.StartElement{Name: xml.Name{Local: "D:collection"}}
		if err := e.EncodeToken(collection); err != nil {
			return err
		}
		if err := e.EncodeToken(collection.End()); err != nil {
			return err
		}
	} else if p.Value != "" {
		if err := e.EncodeToken(xml.CharData([]byte(p.Value))); err != nil {
			return err
		}
	}
	return e.EncodeToken(start.End())
}

func webDAVPropertiesFor(resource *service.WebDAVResource, propfind webDAVPropfindRequest, usage *service.StorageUsage) []webDAVXMLProperty {
	if propfind.Mode == webDAVPropfindPropName {
		return webDAVPropNameProperties()
	}
	return append(webDAVAllProperties(resource), webDAVQuotaProperties(usage)...)
}

func webDAVPropStatsFor(resource *service.WebDAVResource, propfind webDAVPropfindRequest, usage *service.StorageUsage, quotaErr error) []webDAVXMLPropStatus {
	if propfind.Mode == webDAVPropfindProp {
		supported := webDAVSupportedProperties(resource, usage)
		okProps := make([]webDAVXMLProperty, 0, len(propfind.Props))
		missingProps := make([]webDAVXMLProperty, 0)
		failedProps := make([]webDAVXMLProperty, 0)
		for _, name := range propfind.Props {
			if webDAVIsQuotaProperty(name) && quotaErr != nil {
				failedProps = append(failedProps, webDAVXMLProperty{Name: name})
				continue
			}
			property, ok := supported[name]
			if ok {
				okProps = append(okProps, property)
				continue
			}
			missingProps = append(missingProps, webDAVXMLProperty{Name: name})
		}
		stats := make([]webDAVXMLPropStatus, 0, 2)
		if len(okProps) > 0 {
			stats = append(stats, webDAVXMLPropStatus{
				Prop:   webDAVXMLPropSet{Properties: okProps},
				Status: "HTTP/1.1 200 OK",
			})
		}
		if len(failedProps) > 0 {
			stats = append(stats, webDAVXMLPropStatus{
				Prop:   webDAVXMLPropSet{Properties: failedProps},
				Status: "HTTP/1.1 500 Internal Server Error",
			})
		}
		if len(missingProps) > 0 {
			stats = append(stats, webDAVXMLPropStatus{
				Prop:   webDAVXMLPropSet{Properties: missingProps},
				Status: "HTTP/1.1 404 Not Found",
			})
		}
		return stats
	}
	if propfind.Mode == webDAVPropfindAllProp && quotaErr != nil {
		return []webDAVXMLPropStatus{
			{
				Prop:   webDAVXMLPropSet{Properties: webDAVAllProperties(resource)},
				Status: "HTTP/1.1 200 OK",
			},
			{
				Prop:   webDAVXMLPropSet{Properties: webDAVQuotaNameProperties()},
				Status: "HTTP/1.1 500 Internal Server Error",
			},
		}
	}
	return []webDAVXMLPropStatus{{
		Prop:   webDAVXMLPropSet{Properties: webDAVPropertiesFor(resource, propfind, usage)},
		Status: "HTTP/1.1 200 OK",
	}}
}

func webDAVSupportedPropertyNames() []string {
	return []string{
		"creationdate",
		"displayname",
		"getcontentlanguage",
		"getcontentlength",
		"getcontenttype",
		"getetag",
		"getlastmodified",
		"lockdiscovery",
		"resourcetype",
		"supportedlock",
		"quota-used-bytes",
		"quota-available-bytes",
	}
}

func webDAVPropNameProperties() []webDAVXMLProperty {
	names := webDAVSupportedPropertyNames()
	properties := make([]webDAVXMLProperty, 0, len(names))
	for _, name := range names {
		properties = append(properties, webDAVXMLProperty{Name: name})
	}
	return properties
}

func webDAVAllProperties(resource *service.WebDAVResource) []webDAVXMLProperty {
	info := webDAVInfoForResource(resource)
	return []webDAVXMLProperty{
		{Name: "creationdate", Value: info.CreatedAt.UTC().Format(time.RFC3339)},
		{Name: "displayname", Value: info.DisplayName},
		{Name: "getcontentlanguage"},
		{Name: "getcontentlength", Value: fmt.Sprintf("%d", info.Size)},
		{Name: "getcontenttype", Value: info.ContentType},
		{Name: "getetag", Value: info.ETag},
		{Name: "getlastmodified", Value: info.UpdatedAt.UTC().Format(http.TimeFormat)},
		{Name: "lockdiscovery"},
		{Name: "resourcetype", Collection: info.IsDir},
		{Name: "supportedlock"},
	}
}

func webDAVSupportedProperties(resource *service.WebDAVResource, usage *service.StorageUsage) map[string]webDAVXMLProperty {
	properties := append(webDAVAllProperties(resource), webDAVQuotaProperties(usage)...)
	supported := make(map[string]webDAVXMLProperty, len(properties))
	for _, property := range properties {
		supported[property.Name] = property
	}
	return supported
}

func webDAVPropfindNeedsQuota(propfind webDAVPropfindRequest) bool {
	if propfind.Mode == webDAVPropfindPropName {
		return false
	}
	if propfind.Mode == webDAVPropfindAllProp {
		return true
	}
	for _, name := range propfind.Props {
		if webDAVIsQuotaProperty(name) {
			return true
		}
	}
	return false
}

func webDAVIsQuotaProperty(name string) bool {
	return name == "quota-used-bytes" || name == "quota-available-bytes"
}

func webDAVQuotaNameProperties() []webDAVXMLProperty {
	return []webDAVXMLProperty{
		{Name: "quota-used-bytes"},
		{Name: "quota-available-bytes"},
	}
}

func webDAVQuotaProperties(usage *service.StorageUsage) []webDAVXMLProperty {
	if usage == nil {
		return webDAVQuotaNameProperties()
	}
	available := usage.TotalBytes - usage.UsedBytes
	if available < 0 {
		available = 0
	}
	return []webDAVXMLProperty{
		{Name: "quota-used-bytes", Value: fmt.Sprintf("%d", usage.UsedBytes)},
		{Name: "quota-available-bytes", Value: fmt.Sprintf("%d", available)},
	}
}

func parseWebDAVPropfind(body []byte) (webDAVPropfindRequest, error) {
	if len(bytes.TrimSpace(body)) == 0 {
		return webDAVPropfindRequest{Mode: webDAVPropfindAllProp}, nil
	}
	decoder := xml.NewDecoder(bytes.NewReader(body))
	req := webDAVPropfindRequest{}
	depth := 0
	inProp := false
	inInclude := false
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return req, err
		}
		switch t := token.(type) {
		case xml.StartElement:
			depth++
			if depth == 1 {
				if t.Name.Local != "propfind" {
					return req, fmt.Errorf("unexpected root %s", t.Name.Local)
				}
				continue
			}
			if depth == 2 {
				switch t.Name.Local {
				case "allprop":
					if req.Mode == webDAVPropfindPropName {
						return req, fmt.Errorf("multiple propfind modes")
					}
					req.Mode = webDAVPropfindAllProp
				case "propname":
					if req.Mode != "" {
						return req, fmt.Errorf("multiple propfind modes")
					}
					req.Mode = webDAVPropfindPropName
				case "prop":
					if req.Mode == webDAVPropfindAllProp {
						inInclude = true
						continue
					}
					if req.Mode == webDAVPropfindPropName {
						return req, fmt.Errorf("multiple propfind modes")
					}
					req.Mode = webDAVPropfindProp
					inProp = true
				case "include":
					if req.Mode == webDAVPropfindPropName {
						return req, fmt.Errorf("include requires allprop")
					}
					req.Mode = webDAVPropfindAllProp
					inInclude = true
				default:
					return req, fmt.Errorf("unsupported propfind mode %s", t.Name.Local)
				}
				continue
			}
			if (inProp || inInclude) && depth == 3 {
				req.Props = append(req.Props, t.Name.Local)
			}
		case xml.EndElement:
			if depth == 2 && inProp && t.Name.Local == "prop" {
				inProp = false
			}
			if depth == 2 && inInclude && (t.Name.Local == "include" || t.Name.Local == "prop") {
				inInclude = false
			}
			depth--
		}
	}
	if req.Mode == "" {
		req.Mode = webDAVPropfindAllProp
	}
	return req, nil
}

type webDAVResourceInfo struct {
	DisplayName string
	Size        int64
	ContentType string
	ETag        string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	IsDir       bool
}

func webDAVResourceInfoForFile(file *model.File) webDAVResourceInfo {
	contentType := file.MimeType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if file.IsDir {
		contentType = "httpd/unix-directory"
	}
	return webDAVResourceInfo{
		DisplayName: file.Name,
		Size:        file.Size,
		ContentType: contentType,
		ETag:        webDAVETag(file),
		CreatedAt:   file.CreatedAt,
		UpdatedAt:   file.UpdatedAt,
		IsDir:       file.IsDir,
	}
}

func webDAVInfoForResource(resource *service.WebDAVResource) webDAVResourceInfo {
	if resource.Root {
		rootTime := time.Unix(0, 0).UTC()
		return webDAVResourceInfo{
			DisplayName: "",
			ContentType: "httpd/unix-directory",
			ETag:        `"root"`,
			CreatedAt:   rootTime,
			UpdatedAt:   rootTime,
			IsDir:       true,
		}
	}
	return webDAVResourceInfoForFile(resource.File)
}

func webDAVETag(file *model.File) string {
	return fmt.Sprintf(`"%s-%d-%d"`, file.ID, file.UpdatedAt.UnixNano(), file.Size)
}

func webDAVHref(virtualPath string, isDir bool) string {
	if virtualPath == "/" || virtualPath == "" {
		return "/dav/"
	}
	segments := strings.Split(strings.TrimPrefix(path.Clean("/"+strings.TrimPrefix(virtualPath, "/")), "/"), "/")
	var buf bytes.Buffer
	buf.WriteString("/dav/")
	for i, segment := range segments {
		if i > 0 {
			buf.WriteByte('/')
		}
		buf.WriteString(url.PathEscape(segment))
	}
	if isDir {
		buf.WriteByte('/')
	}
	return buf.String()
}
