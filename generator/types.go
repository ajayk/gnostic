// Copyright 2016 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/googleapis/openapi-compiler/jsonschema"
)

/// Type Modeling

// models types that we encounter during traversal that have no named schema
type TypeRequest struct {
	Name         string             // name of type to be created
	PropertyName string             // name of a property that refers to this type
	Schema       *jsonschema.Schema // schema for type
	OneOfWrapper bool               // true if the type wraps "oneOfs"
}

func NewTypeRequest(name string, propertyName string, schema *jsonschema.Schema) *TypeRequest {
	return &TypeRequest{Name: name, PropertyName: propertyName, Schema: schema}
}

// models type properties, eg. fields
type TypeProperty struct {
	Name        string // name of property
	Type        string // type for property (scalar or message type)
	MapType     string // if this property is for a map, the name of the mapped type
	Repeated    bool   // true if this property is repeated (an array)
	Pattern     string // if the property is a pattern property, names must match this pattern.
	Implicit    bool   // true if this property is implied by a pattern or "additional properties" property
	Description string // if present, the "description" field in the schema
}

func (typeProperty *TypeProperty) description() string {
	result := ""
	if typeProperty.Description != "" {
		result += fmt.Sprintf("\t// %+s\n", typeProperty.Description)
	}
	if typeProperty.Repeated {
		result += fmt.Sprintf("\t%s %s repeated %s\n", typeProperty.Name, typeProperty.Type, typeProperty.Pattern)
	} else {
		result += fmt.Sprintf("\t%s %s %s \n", typeProperty.Name, typeProperty.Type, typeProperty.Pattern)
	}
	return result
}

func NewTypeProperty() *TypeProperty {
	return &TypeProperty{}
}

func NewTypePropertyWithNameAndType(name string, typeName string) *TypeProperty {
	return &TypeProperty{Name: name, Type: typeName}
}

func NewTypePropertyWithNameTypeAndPattern(name string, typeName string, pattern string) *TypeProperty {
	return &TypeProperty{Name: name, Type: typeName, Pattern: pattern}
}

// models types
type TypeModel struct {
	Name          string          // type name
	Properties    []*TypeProperty // slice of properties
	Required      []string        // required property names
	OneOfWrapper  bool            // true if this type wraps "oneof" properties
	Open          bool            // open types can have keys outside the specified set
	OpenPatterns  []string        // patterns for properties that we allow
	IsStringArray bool            // ugly override
	IsItemArray   bool            // ugly override
	IsBlob        bool            // ugly override
	IsPair        bool            // type is a name-value pair used to support ordered maps
	PairValueType string          // type for pair values (valid if IsPair == true)
	Description   string          // if present, the "description" field in the schema
}

func (typeModel *TypeModel) AddProperty(property *TypeProperty) {
	if typeModel.Properties == nil {
		typeModel.Properties = make([]*TypeProperty, 0)
	}
	typeModel.Properties = append(typeModel.Properties, property)
}

func (typeModel *TypeModel) description() string {
	result := ""
	if typeModel.Description != "" {
		result += fmt.Sprintf("// %+s\n", typeModel.Description)
	}
	var wrapperinfo string
	if typeModel.OneOfWrapper {
		wrapperinfo = " oneof wrapper"
	}
	result += fmt.Sprintf("%+s%s\n", typeModel.Name, wrapperinfo)
	for _, property := range typeModel.Properties {
		result += property.description()
	}
	return result
}

func NewTypeModel() *TypeModel {
	typeModel := &TypeModel{}
	typeModel.Properties = make([]*TypeProperty, 0)
	return typeModel
}

// models a collection of types that is defined by a schema
type Domain struct {
	TypeModels         map[string]*TypeModel   // models of the types in the collection
	Prefix             string                  // type prefix to use
	Schema             *jsonschema.Schema      // top-level schema
	PatternNames       map[string]string       // a configured mapping from patterns to property names
	ObjectTypeRequests map[string]*TypeRequest // anonymous types implied by type instantiation
	MapTypeRequests    map[string]string       // "NamedObject" types that will be used to implement ordered maps
}

func NewDomain(schema *jsonschema.Schema) *Domain {
	cc := &Domain{}
	cc.TypeModels = make(map[string]*TypeModel, 0)
	cc.PatternNames = make(map[string]string, 0)
	cc.ObjectTypeRequests = make(map[string]*TypeRequest, 0)
	cc.MapTypeRequests = make(map[string]string, 0)
	cc.Schema = schema
	return cc
}

// Returns a capitalized name to use for a generated type
func (domain *Domain) typeNameForStub(stub string) string {
	return domain.Prefix + strings.ToUpper(stub[0:1]) + stub[1:len(stub)]
}

// Returns a capitalized name to use for a generated type based on a JSON reference
func (domain *Domain) typeNameForReference(reference string) string {
	parts := strings.Split(reference, "/")
	first := parts[0]
	last := parts[len(parts)-1]
	if first == "#" {
		return domain.typeNameForStub(last)
	} else {
		return "Schema"
	}
}

// Returns a property name to use for a JSON reference
func (domain *Domain) propertyNameForReference(reference string) *string {
	parts := strings.Split(reference, "/")
	first := parts[0]
	last := parts[len(parts)-1]
	if first == "#" {
		return &last
	} else {
		return nil
	}
	return nil
}

// Determines the item type for arrays defined by a schema
func (domain *Domain) arrayItemTypeForSchema(propertyName string, schema *jsonschema.Schema) string {
	// default
	itemTypeName := "Any"

	if schema.Items != nil {

		if schema.Items.SchemaArray != nil {

			if len(*(schema.Items.SchemaArray)) > 0 {
				ref := (*schema.Items.SchemaArray)[0].Ref
				if ref != nil {
					itemTypeName = domain.typeNameForReference(*ref)
				} else {
					types := (*schema.Items.SchemaArray)[0].Type
					if types == nil {
						// do nothing
					} else if (types.StringArray != nil) && len(*(types.StringArray)) == 1 {
						itemTypeName = (*types.StringArray)[0]
					} else if (types.StringArray != nil) && len(*(types.StringArray)) > 1 {
						itemTypeName = fmt.Sprintf("%+v", types.StringArray)
					} else if types.String != nil {
						itemTypeName = *(types.String)
					} else {
						itemTypeName = "UNKNOWN"
					}
				}
			}

		} else if schema.Items.Schema != nil {
			types := schema.Items.Schema.Type

			if schema.Items.Schema.Ref != nil {
				itemTypeName = domain.typeNameForReference(*schema.Items.Schema.Ref)
			} else if schema.Items.Schema.OneOf != nil {
				// this type is implied by the "oneOf"
				itemTypeName = domain.typeNameForStub(propertyName + "Item")
				domain.ObjectTypeRequests[itemTypeName] =
					NewTypeRequest(itemTypeName, propertyName, schema.Items.Schema)
			} else if types == nil {
				// do nothing
			} else if (types.StringArray != nil) && len(*(types.StringArray)) == 1 {
				itemTypeName = (*types.StringArray)[0]
			} else if (types.StringArray != nil) && len(*(types.StringArray)) > 1 {
				itemTypeName = fmt.Sprintf("%+v", types.StringArray)
			} else if types.String != nil {
				itemTypeName = *(types.String)
			} else {
				itemTypeName = "UNKNOWN"
			}
		}

	}
	return itemTypeName
}

func (domain *Domain) buildTypeProperties(typeModel *TypeModel, schema *jsonschema.Schema) {
	if schema.Properties != nil {
		for _, pair := range *(schema.Properties) {
			propertyName := pair.Name
			propertySchema := pair.Value
			if propertySchema.Ref != nil {
				// the property schema is a reference, so we will add a property with the type of the referenced schema
				propertyTypeName := domain.typeNameForReference(*(propertySchema.Ref))
				typeProperty := NewTypeProperty()
				typeProperty.Name = propertyName
				typeProperty.Type = propertyTypeName
				typeModel.AddProperty(typeProperty)
			} else if propertySchema.Type != nil {
				// the property schema specifies a type, so add a property with the specified type
				if propertySchema.TypeIs("string") {
					typeProperty := NewTypePropertyWithNameAndType(propertyName, "string")
					if propertySchema.Description != nil {
						typeProperty.Description = *propertySchema.Description
					}
					typeModel.AddProperty(typeProperty)
				} else if propertySchema.TypeIs("boolean") {
					typeProperty := NewTypePropertyWithNameAndType(propertyName, "bool")
					if propertySchema.Description != nil {
						typeProperty.Description = *propertySchema.Description
					}
					typeModel.AddProperty(typeProperty)
				} else if propertySchema.TypeIs("number") {
					typeProperty := NewTypePropertyWithNameAndType(propertyName, "float")
					if propertySchema.Description != nil {
						typeProperty.Description = *propertySchema.Description
					}
					typeModel.AddProperty(typeProperty)
				} else if propertySchema.TypeIs("integer") {
					typeProperty := NewTypePropertyWithNameAndType(propertyName, "int")
					if propertySchema.Description != nil {
						typeProperty.Description = *propertySchema.Description
					}
					typeModel.AddProperty(typeProperty)
				} else if propertySchema.TypeIs("object") {
					// the property has an "anonymous" object schema, so define a new type for it and request its creation
					anonymousObjectTypeName := domain.typeNameForStub(propertyName)
					domain.ObjectTypeRequests[anonymousObjectTypeName] =
						NewTypeRequest(anonymousObjectTypeName, propertyName, propertySchema)
					// add a property with the type of the requested type
					typeProperty := NewTypePropertyWithNameAndType(propertyName, anonymousObjectTypeName)
					if propertySchema.Description != nil {
						typeProperty.Description = *propertySchema.Description
					}
					typeModel.AddProperty(typeProperty)
				} else if propertySchema.TypeIs("array") {
					// the property has an array type, so define it as a a repeated property of the specified type
					propertyTypeName := domain.arrayItemTypeForSchema(propertyName, propertySchema)
					typeProperty := NewTypePropertyWithNameAndType(propertyName, propertyTypeName)
					typeProperty.Repeated = true
					if propertySchema.Description != nil {
						typeProperty.Description = *propertySchema.Description
					}
					typeModel.AddProperty(typeProperty)
				} else {
					log.Printf("ignoring %+v, which has an unsupported property type '%+v'", propertyName, propertySchema.Type)
				}
			} else if propertySchema.IsEmpty() {
				// an empty schema can contain anything, so add an accessor for a generic object
				typeName := "Any"
				typeProperty := NewTypePropertyWithNameAndType(propertyName, typeName)
				typeModel.AddProperty(typeProperty)
			} else if propertySchema.OneOf != nil {
				anonymousObjectTypeName := domain.typeNameForStub(propertyName + "Item")
				domain.ObjectTypeRequests[anonymousObjectTypeName] =
					NewTypeRequest(anonymousObjectTypeName, propertyName, propertySchema)
				typeProperty := NewTypePropertyWithNameAndType(propertyName, anonymousObjectTypeName)
				typeModel.AddProperty(typeProperty)
			} else if propertySchema.AnyOf != nil {
				anonymousObjectTypeName := domain.typeNameForStub(propertyName + "Item")
				domain.ObjectTypeRequests[anonymousObjectTypeName] =
					NewTypeRequest(anonymousObjectTypeName, propertyName, propertySchema)
				typeProperty := NewTypePropertyWithNameAndType(propertyName, anonymousObjectTypeName)
				typeModel.AddProperty(typeProperty)
			} else {
				log.Printf("ignoring %s.%s, which has an unrecognized schema:\n%+v", typeModel.Name, propertyName, propertySchema.String())
			}
		}
	}
}

func (domain *Domain) buildTypeRequirements(typeModel *TypeModel, schema *jsonschema.Schema) {
	if schema.Required != nil {
		typeModel.Required = (*schema.Required)
	}
}

func (domain *Domain) buildPatternPropertyAccessors(typeModel *TypeModel, schema *jsonschema.Schema) {
	if schema.PatternProperties != nil {
		typeModel.OpenPatterns = make([]string, 0)
		for _, pair := range *(schema.PatternProperties) {
			propertyPattern := pair.Name
			propertySchema := pair.Value
			typeModel.OpenPatterns = append(typeModel.OpenPatterns, propertyPattern)
			typeName := "Any"
			propertyName := domain.PatternNames[propertyPattern]
			if propertySchema.Ref != nil {
				typeName = domain.typeNameForReference(*propertySchema.Ref)
			}
			propertyTypeName := fmt.Sprintf("Named%s", typeName)
			property := NewTypePropertyWithNameTypeAndPattern(propertyName, propertyTypeName, propertyPattern)
			property.Implicit = true
			property.MapType = typeName
			property.Repeated = true
			domain.MapTypeRequests[property.MapType] = property.MapType
			typeModel.AddProperty(property)
		}
	}
}

func (domain *Domain) buildAdditionalPropertyAccessors(typeModel *TypeModel, schema *jsonschema.Schema) {
	if schema.AdditionalProperties != nil {
		if schema.AdditionalProperties.Boolean != nil {
			if *schema.AdditionalProperties.Boolean == true {
				typeModel.Open = true
				propertyName := "additionalProperties"
				typeName := "NamedAny"
				property := NewTypePropertyWithNameAndType(propertyName, typeName)
				property.Implicit = true
				property.MapType = "Any"
				property.Repeated = true
				domain.MapTypeRequests[property.MapType] = property.MapType
				typeModel.AddProperty(property)
				return
			}
		} else if schema.AdditionalProperties.Schema != nil {
			typeModel.Open = true
			schema := schema.AdditionalProperties.Schema
			if schema.Ref != nil {
				propertyName := "additionalProperties"
				mapType := domain.typeNameForReference(*schema.Ref)
				typeName := fmt.Sprintf("Named%s", mapType)
				property := NewTypePropertyWithNameAndType(propertyName, typeName)
				property.Implicit = true
				property.MapType = mapType
				property.Repeated = true
				domain.MapTypeRequests[property.MapType] = property.MapType
				typeModel.AddProperty(property)
				return
			} else if schema.Type != nil {
				typeName := *schema.Type.String
				if typeName == "string" {
					propertyName := "additionalProperties"
					typeName := "NamedString"
					property := NewTypePropertyWithNameAndType(propertyName, typeName)
					property.Implicit = true
					property.MapType = "string"
					property.Repeated = true
					domain.MapTypeRequests[property.MapType] = property.MapType
					typeModel.AddProperty(property)
					return
				} else if typeName == "array" {
					if schema.Items != nil {
						itemType := *schema.Items.Schema.Type.String
						if itemType == "string" {
							propertyName := "additionalProperties"
							typeName := "NamedStringArray"
							property := NewTypePropertyWithNameAndType(propertyName, typeName)
							property.Implicit = true
							property.MapType = "StringArray"
							property.Repeated = true
							domain.MapTypeRequests[property.MapType] = property.MapType
							typeModel.AddProperty(property)
							return
						}
					}
				}
			} else if schema.OneOf != nil {
				propertyTypeName := domain.typeNameForStub(typeModel.Name + "Item")
				propertyName := "additionalProperties"
				typeName := fmt.Sprintf("Named%s", propertyTypeName)
				property := NewTypePropertyWithNameAndType(propertyName, typeName)
				property.Implicit = true
				property.MapType = propertyTypeName
				property.Repeated = true
				domain.MapTypeRequests[property.MapType] = property.MapType
				typeModel.AddProperty(property)

				domain.ObjectTypeRequests[propertyTypeName] =
					NewTypeRequest(propertyTypeName, propertyName, schema)
			}
		}
	}
}

func (domain *Domain) buildOneOfAccessors(typeModel *TypeModel, schema *jsonschema.Schema) {
	oneOfs := schema.OneOf
	if oneOfs == nil {
		return
	}
	typeModel.Open = true
	typeModel.OneOfWrapper = true
	for _, oneOf := range *oneOfs {
		if oneOf.Ref != nil {
			ref := *oneOf.Ref
			typeName := domain.typeNameForReference(ref)
			propertyName := domain.propertyNameForReference(ref)

			if propertyName != nil {
				typeProperty := NewTypePropertyWithNameAndType(*propertyName, typeName)
				typeModel.AddProperty(typeProperty)
			}
		} else if oneOf.Type != nil && oneOf.Type.String != nil && *oneOf.Type.String == "boolean" {
			typeProperty := NewTypePropertyWithNameAndType("boolean", "bool")
			typeModel.AddProperty(typeProperty)
		} else {
			log.Printf("Unsupported oneOf:\n%+v", oneOf.String())
		}

	}
}

func schemaIsContainedInArray(s1 *jsonschema.Schema, s2 *jsonschema.Schema) bool {
	if s2.TypeIs("array") {
		if s2.Items.Schema != nil {
			if s1.IsEqual(s2.Items.Schema) {
				return true
			} else {
				return false
			}
		} else {
			return false
		}
	} else {
		return false
	}
}

func (domain *Domain) addAnonymousAccessorForSchema(
	typeModel *TypeModel,
	schema *jsonschema.Schema,
	repeated bool) {
	ref := schema.Ref
	if ref != nil {
		typeName := domain.typeNameForReference(*ref)
		propertyName := domain.propertyNameForReference(*ref)
		if propertyName != nil {
			property := NewTypePropertyWithNameAndType(*propertyName, typeName)
			property.Repeated = true
			typeModel.AddProperty(property)
			typeModel.IsItemArray = true
		}
	} else {
		typeName := "string"
		propertyName := "value"
		property := NewTypePropertyWithNameAndType(propertyName, typeName)
		property.Repeated = true
		typeModel.AddProperty(property)
		typeModel.IsStringArray = true
	}
}

func (domain *Domain) buildAnyOfAccessors(typeModel *TypeModel, schema *jsonschema.Schema) {
	anyOfs := schema.AnyOf
	if anyOfs == nil {
		return
	}
	if len(*anyOfs) == 2 {
		if schemaIsContainedInArray((*anyOfs)[0], (*anyOfs)[1]) {
			log.Printf("ARRAY OF %+v", (*anyOfs)[0].String())
			schema := (*anyOfs)[0]
			domain.addAnonymousAccessorForSchema(typeModel, schema, true)
		} else if schemaIsContainedInArray((*anyOfs)[1], (*anyOfs)[0]) {
			log.Printf("ARRAY OF %+v", (*anyOfs)[1].String())
			schema := (*anyOfs)[1]
			domain.addAnonymousAccessorForSchema(typeModel, schema, true)
		} else {
			for _, anyOf := range *anyOfs {
				ref := anyOf.Ref
				if ref != nil {
					typeName := domain.typeNameForReference(*ref)
					propertyName := domain.propertyNameForReference(*ref)
					if propertyName != nil {
						property := NewTypePropertyWithNameAndType(*propertyName, typeName)
						typeModel.AddProperty(property)
					}
				} else {
					typeName := "bool"
					propertyName := "boolean"
					property := NewTypePropertyWithNameAndType(propertyName, typeName)
					typeModel.AddProperty(property)
				}
			}
		}
	} else {
		log.Printf("Unhandled anyOfs:\n%s", schema.String())
	}
}

func (domain *Domain) buildDefaultAccessors(typeModel *TypeModel, schema *jsonschema.Schema) {
	typeModel.Open = true
	propertyName := "additionalProperties"
	typeName := "NamedAny"
	property := NewTypePropertyWithNameAndType(propertyName, typeName)
	property.MapType = "Any"
	property.Repeated = true
	domain.MapTypeRequests[property.MapType] = property.MapType
	typeModel.AddProperty(property)
}

func (domain *Domain) buildTypeForDefinition(
	typeName string,
	propertyName string,
	schema *jsonschema.Schema) *TypeModel {
	if (schema.Type == nil) || (*schema.Type.String == "object") {
		return domain.buildTypeForDefinitionObject(typeName, propertyName, schema)
	} else {
		return nil
	}
}

func (domain *Domain) buildTypeForDefinitionObject(
	typeName string,
	propertyName string,
	schema *jsonschema.Schema) *TypeModel {
	typeModel := NewTypeModel()
	typeModel.Name = typeName
	if schema.IsEmpty() {
		domain.buildDefaultAccessors(typeModel, schema)
	} else {
		if schema.Description != nil {
			typeModel.Description = *schema.Description
		}
		domain.buildTypeProperties(typeModel, schema)
		domain.buildTypeRequirements(typeModel, schema)
		domain.buildPatternPropertyAccessors(typeModel, schema)
		domain.buildAdditionalPropertyAccessors(typeModel, schema)
		domain.buildOneOfAccessors(typeModel, schema)
		domain.buildAnyOfAccessors(typeModel, schema)
	}
	return typeModel
}

func (domain *Domain) build() {
	// create a type for the top-level schema
	typeName := domain.Prefix + "Document"
	typeModel := NewTypeModel()
	typeModel.Name = typeName
	domain.buildTypeProperties(typeModel, domain.Schema)
	domain.buildTypeRequirements(typeModel, domain.Schema)
	domain.buildPatternPropertyAccessors(typeModel, domain.Schema)
	domain.buildAdditionalPropertyAccessors(typeModel, domain.Schema)
	domain.buildOneOfAccessors(typeModel, domain.Schema)
	domain.buildAnyOfAccessors(typeModel, domain.Schema)
	domain.TypeModels[typeName] = typeModel

	// create a type for each object defined in the schema
	for _, pair := range *(domain.Schema.Definitions) {
		definitionName := pair.Name
		definitionSchema := pair.Value
		typeName := domain.typeNameForStub(definitionName)
		typeModel := domain.buildTypeForDefinition(typeName, definitionName, definitionSchema)
		if typeModel != nil {
			domain.TypeModels[typeName] = typeModel
		}
	}

	// iterate over anonymous object types to be instantiated and generate a type for each
	for typeName, typeRequest := range domain.ObjectTypeRequests {
		domain.TypeModels[typeRequest.Name] =
			domain.buildTypeForDefinitionObject(typeName, typeRequest.PropertyName, typeRequest.Schema)
	}

	// iterate over map item types to be instantiated and generate a type for each
	mapTypeNames := make([]string, 0)
	for mapTypeName, _ := range domain.MapTypeRequests {
		mapTypeNames = append(mapTypeNames, mapTypeName)
	}
	sort.Strings(mapTypeNames)

	for _, mapTypeName := range mapTypeNames {
		typeName := "Named" + strings.Title(mapTypeName)
		typeModel := NewTypeModel()
		typeModel.Name = typeName
		typeModel.Description = fmt.Sprintf(
			"Automatically-generated message used to represent maps of %s as ordered (name,value) pairs.",
			mapTypeName)
		typeModel.IsPair = true
		typeModel.PairValueType = mapTypeName

		nameProperty := NewTypeProperty()
		nameProperty.Name = "name"
		nameProperty.Type = "string"
		nameProperty.Description = "Map key"
		typeModel.AddProperty(nameProperty)

		valueProperty := NewTypeProperty()
		valueProperty.Name = "value"
		valueProperty.Type = mapTypeName
		valueProperty.Description = "Mapped value"
		typeModel.AddProperty(valueProperty)

		domain.TypeModels[typeName] = typeModel
	}

	// add a type for string arrays
	stringArrayType := NewTypeModel()
	stringArrayType.Name = "StringArray"
	stringProperty := NewTypeProperty()
	stringProperty.Name = "value"
	stringProperty.Type = "string"
	stringProperty.Repeated = true
	stringArrayType.AddProperty(stringProperty)
	domain.TypeModels[stringArrayType.Name] = stringArrayType

	// add a type for "Any"
	anyType := NewTypeModel()
	anyType.Name = "Any"
	anyType.Open = true
	anyType.IsBlob = true
	valueProperty := NewTypeProperty()
	valueProperty.Name = "value"
	valueProperty.Type = "blob"
	anyType.AddProperty(valueProperty)
	domain.TypeModels[anyType.Name] = anyType
}

func (domain *Domain) sortedTypeNames() []string {
	typeNames := make([]string, 0)
	for typeName, _ := range domain.TypeModels {
		typeNames = append(typeNames, typeName)
	}
	sort.Strings(typeNames)
	return typeNames
}

func (domain *Domain) description() string {
	typeNames := domain.sortedTypeNames()
	result := ""
	for _, typeName := range typeNames {
		result += domain.TypeModels[typeName].description()
	}
	return result
}