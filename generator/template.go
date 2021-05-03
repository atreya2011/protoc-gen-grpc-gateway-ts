package generator

import (
	"bytes"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"text/template"

	log "github.com/sirupsen/logrus"

	"github.com/Masterminds/sprig"
	"github.com/iancoleman/strcase"

	"github.com/atreya2011/protoc-gen-grpc-gateway-ts/data"
	"github.com/atreya2011/protoc-gen-grpc-gateway-ts/registry"
)

const tmpl = `
{{- define "dependencies"}}{{range removeWellKnownTypes .}}
import * as {{.ModuleIdentifier}} from "{{.SourceFile}}"{{end}}
{{end}}

{{define "enums"}}{{range .}}
export enum {{.Name}} {
{{- range .Values}}
  {{.}} = "{{.}}",
{{- end}}
}
{{end}}{{end}}

{{define "messages"}}
{{range .}}
{{- if .HasOneOfFields}}
type Base{{.Name}} = {
{{- range .NonOneOfFields}}
  {{fieldName .Name}}?: {{tsType .}}
{{- end}}
}

export type {{.Name}} = Base{{.Name}}
{{range $groupId, $fields := .OneOfFieldsGroups}}  & OneOf<{ {{range $index, $field := $fields}}{{fieldName $field.Name}}: {{tsType $field}}{{if (lt (add $index 1) (len $fields))}}; {{end}}{{end}} }>
{{end}}
{{- else -}}
export type {{.Name}} = {
{{- range .Fields}}
  {{fieldName .Name}}?: {{tsType .}}
{{- end}}
}
{{end}}
{{end}}{{end}}

{{define "services"}}{{range .}}export class {{.Name}} {
{{- range .Methods}}  
{{- if .ServerStreaming }}
  static {{.Name}}(req: {{tsType .Input}}, entityNotifier?: fm.NotifyStreamEntityArrival<{{tsType .Output}}>, initReq?: fm.InitReq): Promise<void> {
    return fm.fetchStreamingRequest<{{tsType .Input}}, {{tsType .Output}}>(` + "`{{renderURL .}}`" + `, entityNotifier, {...initReq, {{buildInitReq .}}})
  }
{{- else }}
  static {{.Name}}(req: {{tsType .Input}}, initReq?: fm.InitReq): Promise<{{tsType .Output}}> {
    return fm.fetchReq<{{tsType .Input}}, {{tsType .Output}}>(` + "`{{renderURL .}}`" + `, {...initReq, {{buildInitReq .}}})
  }
{{- end}}
{{- end}}
}
{{end}}{{end}}

/*
* This file is a generated Typescript file for GRPC Gateway, DO NOT MODIFY
*/
{{if .Dependencies}}{{- include "dependencies" .StableDependencies -}}{{end}}
{{- if .NeedsOneOfSupport}}
type Absent<T, K extends keyof T> = { [k in Exclude<keyof T, K>]?: undefined };
type OneOf<T> =
  | { [k in keyof T]?: undefined }
  | (
    keyof T extends infer K ?
      (K extends string & keyof T ? { [k in K]: T[K] } & Absent<T, K>
        : never)
    : never);
{{end}}
{{- if .Enums}}{{include "enums" .Enums}}{{end}}
{{- if .Messages}}{{include "messages" .Messages}}{{end}}
{{- if .Services}}{{include "services" .Services}}{{end}}
`

const fetchTmpl = `
/*
* This file is a generated Typescript file for GRPC Gateway, DO NOT MODIFY
*/

export interface InitReq extends RequestInit {
  pathPrefix?: string
}

export function fetchReq<I, O>(path: string, init?: InitReq): Promise<O> {
  const {pathPrefix, ...req} = init || {}

  const url = pathPrefix ? ` + "`${pathPrefix}${path}`" + ` : path

  return fetch(url, req).then(r => r.json()) as Promise<O>
}

// NotifyStreamEntityArrival is a callback that will be called on streaming entity arrival
export type NotifyStreamEntityArrival<T> = (resp: T) => void

/**
 * fetchStreamingRequest is able to handle grpc-gateway server side streaming call
 * it takes NotifyStreamEntityArrival that lets users respond to entity arrival during the call
 * all entities will be returned as an array after the call finishes.
 **/
export async function fetchStreamingRequest<S, R>(path: string, callback?: NotifyStreamEntityArrival<R>, init?: InitReq) {
  const {pathPrefix, ...req} = init || {}
  const url = pathPrefix ?` + "`${pathPrefix}${path}`" + ` : path
  const result = await fetch(url, req)
  // needs to use the .ok to check the status of HTTP status code
  // http other than 200 will not throw an error, instead the .ok will become false.
  // see https://developer.mozilla.org/en-US/docs/Web/API/Fetch_API/Using_Fetch#
  if (!result.ok) {
    const resp = await result.json()
    const errMsg = resp.error && resp.error.message ? resp.error.message : ""
    throw new Error(errMsg)
  }

  if (!result.body) {
    throw new Error("response doesnt have a body")
  }

  await result.body
    .pipeThrough(new TextDecoderStream())
    .pipeThrough<R>(getNewLineDelimitedJSONDecodingStream<R>())
    .pipeTo(getNotifyEntityArrivalSink((e: R) => {
      if (callback) {
        callback(e)
      }
    }))

  // wait for the streaming to finish and return the success respond
  return
}

/**
 * JSONStringStreamController represents the transform controller that's able to transform the incoming
 * new line delimited json content stream into entities and able to push the entity to the down stream
 */
interface JSONStringStreamController<T> extends TransformStreamDefaultController {
  buf?: string
  pos?: number
  enqueue: (s: T) => void
}

/**
 * getNewLineDelimitedJSONDecodingStream returns a TransformStream that's able to handle new line delimited json stream content into parsed entities
 */
function getNewLineDelimitedJSONDecodingStream<T>(): TransformStream<string, T> {
  return new TransformStream({
    start(controller: JSONStringStreamController<T>) {
      controller.buf = ''
      controller.pos = 0
    },

    transform(chunk: string, controller: JSONStringStreamController<T>) {
      if (controller.buf === undefined) {
        controller.buf = ''
      }
      if (controller.pos === undefined) {
        controller.pos = 0
      }
      controller.buf += chunk
      while (controller.pos < controller.buf.length) {
        if (controller.buf[controller.pos] === '\n') {
          const line = controller.buf.substring(0, controller.pos)
          const response = JSON.parse(line)
          controller.enqueue(response.result)
          controller.buf = controller.buf.substring(controller.pos + 1)
          controller.pos = 0
        } else {
          ++controller.pos
        }
      }
    }
  })

}

/**
 * getNotifyEntityArrivalSink takes the NotifyStreamEntityArrival callback and return
 * a sink that will call the callback on entity arrival
 * @param notifyCallback
 */
function getNotifyEntityArrivalSink<T>(notifyCallback: NotifyStreamEntityArrival<T>) {
  return new WritableStream<T>({
    write(entity: T) {
      notifyCallback(entity)
    }
  })
}

type Primitive = string | boolean | number;
type RequestPayload = Record<string, unknown>;
type FlattenedRequestPayload = Record<string, Primitive | Array<Primitive>>;

/**
 * Checks if given value is a plain object
 * Logic copied and adapted from below source: 
 * https://github.com/char0n/ramda-adjunct/blob/master/src/isPlainObj.js
 * @param  {unknown} value
 * @return {boolean}
 */
function isPlainObject(value: unknown): boolean {
  const isObject =
    Object.prototype.toString.call(value).slice(8, -1) === "Object";
  const isObjLike = value !== null && isObject;

  if (!isObjLike || !isObject) {
    return false;
  }

  const proto = Object.getPrototypeOf(value);

  const hasObjectConstructor =
    typeof proto === "object" &&
    proto.constructor === Object.prototype.constructor;

  return hasObjectConstructor;
}

/**
 * Checks if given value is of a primitive type
 * @param  {unknown} value
 * @return {boolean}
 */
function isPrimitive(value: unknown): boolean {
  return ["string", "number", "boolean"].some(t => typeof value === t);
}

/**
 * Checks if given primitive is zero-value
 * @param  {Primitive} value
 * @return {boolean}
 */
function isZeroValuePrimitive(value: Primitive): boolean {
  return value === false || value === 0 || value === "";
}

/**
 * Flattens a deeply nested request payload and returns an object
 * with only primitive values and non-empty array of primitive values
 * as per https://github.com/googleapis/googleapis/blob/master/google/api/http.proto
 * @param  {RequestPayload} requestPayload
 * @param  {String} path
 * @return {FlattenedRequestPayload>}
 */
function flattenRequestPayload<T extends RequestPayload>(
  requestPayload: T,
  path: string = ""
): FlattenedRequestPayload {
  return Object.keys(requestPayload).reduce(
    (acc: T, key: string): T => {
      const value = requestPayload[key];
      const newPath = path ? [path, key].join(".") : key;

      const isNonEmptyPrimitiveArray =
        Array.isArray(value) &&
        value.every(v => isPrimitive(v)) &&
        value.length > 0;

      const isNonZeroValuePrimitive =
        isPrimitive(value) && !isZeroValuePrimitive(value as Primitive);

      let objectToMerge = {};

      if (isPlainObject(value)) {
        objectToMerge = flattenRequestPayload(value as RequestPayload, newPath);
      } else if (isNonZeroValuePrimitive || isNonEmptyPrimitiveArray) {
        objectToMerge = { [newPath]: value };
      }

      return { ...acc, ...objectToMerge };
    },
    {} as T
  ) as FlattenedRequestPayload;
}

/**
 * Renders a deeply nested request payload into a string of URL search
 * parameters by first flattening the request payload and then removing keys
 * which are already present in the URL path.
 * @param  {RequestPayload} requestPayload
 * @param  {string[]} urlPathParams
 * @return {string}
 */
export function renderURLSearchParams<T extends RequestPayload>(
  requestPayload: T,
  urlPathParams: string[] = []
): string {
  const flattenedRequestPayload = flattenRequestPayload(requestPayload);

  const urlSearchParams = Object.keys(flattenedRequestPayload).reduce(
    (acc: string[][], key: string): string[][] => {
      // key should not be present in the url path as a parameter
      const value = flattenedRequestPayload[key];
      if (urlPathParams.find(f => f === key)) {
        return acc;
      }
      return Array.isArray(value)
        ? [...acc, ...value.map(m => [key, m.toString()])]
        : (acc = [...acc, [key, value.toString()]]);
    },
    [] as string[][]
  );

  return new URLSearchParams(urlSearchParams).toString();
}
`

// GetTemplate gets the templates to for the typescript file
func GetTemplate(r *registry.Registry) *template.Template {
	t := template.New("file")
	t = t.Funcs(sprig.TxtFuncMap())

	t = t.Funcs(template.FuncMap{
		"include": include(t),
		"tsType": func(fieldType data.Type) string {
			return tsType(r, fieldType)
		},
		"renderURL":    renderURL(r),
		"buildInitReq": buildInitReq,
		"fieldName":    fieldName(r),
	})

	t = template.Must(t.Parse(tmpl))
	return t
}

func fieldName(r *registry.Registry) func(name string) string {
	return func(name string) string {
		if r.UseProtoNames {
			return name
		}

		return strcase.ToLowerCamel(name)
	}
}

func renderURL(r *registry.Registry) func(method data.Method) string {
	fieldNameFn := fieldName(r)
	return func(method data.Method) string {
		methodURL := method.URL
		reg := regexp.MustCompile("{([^}]+)}")
		matches := reg.FindAllStringSubmatch(methodURL, -1)
		fieldsInPath := make([]string, 0, len(matches))
		if len(matches) > 0 {
			log.Debugf("url matches %v", matches)
			for _, m := range matches {
				expToReplace := m[0]
				fieldName := fieldNameFn(m[1])
				part := fmt.Sprintf(`${req["%s"]}`, fieldName)
				methodURL = strings.ReplaceAll(methodURL, expToReplace, part)
				fieldsInPath = append(fieldsInPath, fmt.Sprintf(`"%s"`, fieldName))
			}
		}
		urlPathParams := fmt.Sprintf("[%s]", strings.Join(fieldsInPath, ", "))

		if !method.ClientStreaming && method.HTTPMethod == "GET" {
			// parse the url to check for query string
			parsedURL, err := url.Parse(methodURL)
			if err != nil {
				return methodURL
			}
			renderURLSearchParamsFn := fmt.Sprintf("${fm.renderURLSearchParams(req, %s)}", urlPathParams)
			// prepend "&" if query string is present otherwise prepend "?"
			// trim leading "&" if present before prepending it
			if parsedURL.RawQuery != "" {
				methodURL = strings.TrimRight(methodURL, "&") + "&" + renderURLSearchParamsFn
			} else {
				methodURL += "?" + renderURLSearchParamsFn
			}
		}

		return methodURL
	}
}

func buildInitReq(method data.Method) string {
	httpMethod := method.HTTPMethod
	m := `method: "` + httpMethod + `"`
	fields := []string{m}
	if method.HTTPRequestBody == nil || *method.HTTPRequestBody == "*" {
		fields = append(fields, "body: JSON.stringify(req)")
	} else if *method.HTTPRequestBody != "" {
		fields = append(fields, `body: JSON.stringify(req["`+*method.HTTPRequestBody+`"])`)
	}

	return strings.Join(fields, ", ")

}

// GetFetchModuleTemplate returns the go template for fetch module
func GetFetchModuleTemplate() *template.Template {
	t := template.New("fetch")
	return template.Must(t.Parse(fetchTmpl))
}

// include is the include template functions copied from
// copied from: https://github.com/helm/helm/blob/8648ccf5d35d682dcd5f7a9c2082f0aaf071e817/pkg/engine/engine.go#L147-L154
func include(t *template.Template) func(name string, data interface{}) (string, error) {
	return func(name string, data interface{}) (string, error) {
		buf := bytes.NewBufferString("")
		if err := t.ExecuteTemplate(buf, name, data); err != nil {
			return "", err
		}
		return buf.String(), nil
	}
}

func tsType(r *registry.Registry, fieldType data.Type) string {
	info := fieldType.GetType()
	typeInfo, ok := r.Types[info.Type]
	if ok && typeInfo.IsMapEntry {
		keyType := tsType(r, typeInfo.KeyType)
		valueType := tsType(r, typeInfo.ValueType)

		return fmt.Sprintf("{[key: %s]: %s}", keyType, valueType)
	}

	typeStr := ""
	if strings.Index(info.Type, ".") != 0 {
		typeStr = mapScalaType(info.Type)
	} else if !info.IsExternal {
		typeStr = typeInfo.PackageIdentifier
	} else {
		typeStr = mapWellKnownType(info.Type)
		if typeStr == "" {
			typeStr = data.GetModuleName(typeInfo.Package, typeInfo.File) + "." + typeInfo.PackageIdentifier
		}
	}

	if info.IsRepeated {
		typeStr += "[]"
	}
	return typeStr
}

func mapScalaType(protoType string) string {
	switch protoType {
	case "uint64", "sint64", "int64", "fixed64", "sfixed64", "string":
		return "string"
	case "float", "double", "int32", "sint32", "uint32", "fixed32", "sfixed32":
		return "number"
	case "bool":
		return "boolean"
	case "bytes":
		return "Uint8Array"
	}

	return ""
}

func mapWellKnownType(wellKnownType string) string {
	switch wellKnownType {
	case ".google.protobuf.Any":
		return "any"
	case ".google.protobuf.Empty":
		return "Record<never, never>"
	case ".google.protobuf.Timestamp":
		return "Date"
	}
	return ""
}
