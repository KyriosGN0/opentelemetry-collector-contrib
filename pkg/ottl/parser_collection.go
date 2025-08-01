// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package ottl // import "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl"

import (
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.uber.org/zap"
)

// StatementsGetter represents a set of statements to be parsed.
//
// Experimental: *NOTE* this API is subject to change or removal in the future.
type StatementsGetter interface {
	// GetStatements retrieves the OTTL statements to be parsed
	GetStatements() []string
}

// NewStatementsGetter creates a new StatementsGetter.
//
// Experimental: *NOTE* this API is subject to change or removal in the future.
func NewStatementsGetter(statements []string) StatementsGetter {
	return defaultOTTLGetter(statements)
}

// ConditionsGetter represents a set of conditions to be parsed.
//
// Experimental: *NOTE* this API is subject to change or removal in the future.
type ConditionsGetter interface {
	// GetConditions retrieves the OTTL conditions to be parsed
	GetConditions() []string
}

// NewConditionsGetter creates a new ConditionsGetter.
//
// Experimental: *NOTE* this API is subject to change or removal in the future.
func NewConditionsGetter(conditions []string) ConditionsGetter {
	return defaultOTTLGetter(conditions)
}

// ValueExpressionsGetter represents a set of value expressions to be parsed.
//
// Experimental: *NOTE* this API is subject to change or removal in the future.
type ValueExpressionsGetter interface {
	// GetValueExpressions retrieves the OTTL value expressions to be parsed
	GetValueExpressions() []string
}

// NewValueExpressionsGetter creates a new ValueExpressionsGetter.
//
// Experimental: *NOTE* this API is subject to change or removal in the future.
func NewValueExpressionsGetter(expressions []string) ValueExpressionsGetter {
	return defaultOTTLGetter(expressions)
}

type defaultOTTLGetter []string

func (d defaultOTTLGetter) GetStatements() []string {
	return d
}

func (d defaultOTTLGetter) GetConditions() []string {
	return d
}

func (d defaultOTTLGetter) GetValueExpressions() []string {
	return d
}

// ParserCollection is a configurable set of ottl.Parser that can handle multiple OTTL contexts
// parsings, inferring the context, choosing the right parser for the given statements, and
// transforming the parsed ottl.Statement[K] slice into a common result of type R.
//
// Experimental: *NOTE* this API is subject to change or removal in the future.
type ParserCollection[R any] struct {
	contextParsers            map[string]*ParserCollectionContextParser[R]
	contextInferrer           contextInferrer
	contextInferrerCandidates map[string]*priorityContextInferrerCandidate
	candidatesLowerContexts   map[string][]string
	modifiedLogging           bool
	Settings                  component.TelemetrySettings
	ErrorMode                 ErrorMode
}

// ParserCollectionOption is a configurable ParserCollection option.
//
// Experimental: *NOTE* this API is subject to change or removal in the future.
type ParserCollectionOption[R any] func(*ParserCollection[R]) error

// NewParserCollection creates a new ParserCollection.
//
// Experimental: *NOTE* this API is subject to change or removal in the future.
func NewParserCollection[R any](
	settings component.TelemetrySettings,
	options ...ParserCollectionOption[R],
) (*ParserCollection[R], error) {
	contextInferrerCandidates := map[string]*priorityContextInferrerCandidate{}
	pc := &ParserCollection[R]{
		Settings:                  settings,
		contextParsers:            map[string]*ParserCollectionContextParser[R]{},
		contextInferrer:           newPriorityContextInferrer(settings, contextInferrerCandidates),
		contextInferrerCandidates: contextInferrerCandidates,
		candidatesLowerContexts:   map[string][]string{},
	}

	for _, op := range options {
		err := op(pc)
		if err != nil {
			return nil, err
		}
	}

	return pc, nil
}

// ParsedStatementsConverter is a function that converts the parsed ottl.Statement[K] into
// a common representation to all parser collection contexts passed through WithParserCollectionContext.
// Given each parser has its own transform context type, they must agree on a common type [R]
// so it can be returned by the ParserCollection.ParseStatements and ParserCollection.ParseStatementsWithContext
// functions.
//
// Experimental: *NOTE* this API is subject to change or removal in the future.
type ParsedStatementsConverter[K any, R any] func(collection *ParserCollection[R], statements StatementsGetter, parsedStatements []*Statement[K]) (R, error)

// ParsedConditionsConverter is a function that converts the parsed ottl.Condition[K] into
// a common representation to all parser collection contexts passed through WithParserCollectionContext.
// Given each parser has its own transform context type, they must agree on a common type [R]
// so it can be returned by the ParserCollection.ParseConditions and ParserCollection.ParseConditionsWithContext
// functions.
//
// Experimental: *NOTE* this API is subject to change or removal in the future.
type ParsedConditionsConverter[K any, R any] func(collection *ParserCollection[R], conditions ConditionsGetter, parsedConditions []*Condition[K]) (R, error)

// ParsedValueExpressionsConverter is a function that converts the parsed ottl.ValueExpression[K] into
// a representation common to all parser collection contexts passed through WithParserCollectionContext.
// Given each parser has its own transform context type, they must agree on a common type [R]
// so it can be returned by the ParserCollection.ParseValueExpressions and ParserCollection.ParseValueExpressionsWithContext
// functions.
//
// Experimental: *NOTE* this API is subject to change or removal in the future.
type ParsedValueExpressionsConverter[K any, R any] func(collection *ParserCollection[R], expressions ValueExpressionsGetter, parsedValueExpressions []*ValueExpression[K]) (R, error)

func newNopParsedStatementsConverter[K any]() ParsedStatementsConverter[K, any] {
	return func(
		_ *ParserCollection[any],
		_ StatementsGetter,
		parsedStatements []*Statement[K],
	) (any, error) {
		return parsedStatements, nil
	}
}

func newNopParsedConditionsConverter[K any]() ParsedConditionsConverter[K, any] {
	return func(
		_ *ParserCollection[any],
		_ ConditionsGetter,
		parsedConditions []*Condition[K],
	) (any, error) {
		return parsedConditions, nil
	}
}

func newNopParsedValueExpressionsConverter[K any]() ParsedValueExpressionsConverter[K, any] {
	return func(
		_ *ParserCollection[any],
		_ ValueExpressionsGetter,
		parsedValueExpressions []*ValueExpression[K],
	) (any, error) {
		return parsedValueExpressions, nil
	}
}

type (
	// ParserCollectionContextOption is a configurable ParserCollectionContext option.
	//
	// Experimental: *NOTE* this API is subject to change or removal in the future.
	ParserCollectionContextOption[K, R any] func(*ParserCollectionContextParser[R], *Parser[K])

	// parserCollectionContextParserFunc is the internal generic type that parses the given []string
	// returned from a getter G into type [R] using the specified context's OTTL parser.
	// The provided context must be supported by the ParserCollection, otherwise an error is returned.
	// If the OTTL Path does not provide their Path.Context value, the prependPathsContext argument should be set to true,
	// so it rewrites the OTTL prepending the missing paths contexts.
	parserCollectionContextParserFunc[R any, G any] func(collection *ParserCollection[R], context string, getter G, prependPathsContext bool) (R, error)
	// ParserCollectionContextParser is a struct that holds the converters for parsing statements and conditions
	// into a common representation of type [R].
	//
	// Experimental: *NOTE* this API is subject to change or removal in the future.
	ParserCollectionContextParser[R any] struct {
		parseStatements       parserCollectionContextParserFunc[R, StatementsGetter]
		parseConditions       parserCollectionContextParserFunc[R, ConditionsGetter]
		parseValueExpressions parserCollectionContextParserFunc[R, ValueExpressionsGetter]
	}
)

// createConditionsParserWithConverter is a method to create the necessary parser wrapper and shadowing the K type.
func createConditionsParserWithConverter[K, R any](converter ParsedConditionsConverter[K, R], parser *Parser[K]) parserCollectionContextParserFunc[R, ConditionsGetter] {
	return func(pc *ParserCollection[R], context string, conditions ConditionsGetter, prependPathsContext bool) (R, error) {
		var err error
		var parsingConditions []string
		if prependPathsContext {
			originalConditions := conditions.GetConditions()
			parsingConditions = make([]string, 0, len(originalConditions))
			for _, cond := range originalConditions {
				prependedCondition, prependErr := parser.prependContextToConditionPaths(context, cond)
				if prependErr != nil {
					err = prependErr
					break
				}
				parsingConditions = append(parsingConditions, prependedCondition)
			}
			if err != nil {
				return *new(R), err
			}
			if pc.modifiedLogging {
				pc.logModifications(originalConditions, parsingConditions)
			}
		} else {
			parsingConditions = conditions.GetConditions()
		}
		parsedConditions, err := parser.ParseConditions(parsingConditions)
		if err != nil {
			return *new(R), err
		}
		return converter(
			pc,
			conditions,
			parsedConditions,
		)
	}
}

// createValueExpressionsParserWithConverter is a method to create the necessary parser wrapper and shadowing the K type.
func createValueExpressionsParserWithConverter[K, R any](converter ParsedValueExpressionsConverter[K, R], parser *Parser[K]) parserCollectionContextParserFunc[R, ValueExpressionsGetter] {
	return func(pc *ParserCollection[R], context string, expressions ValueExpressionsGetter, prependPathsContext bool) (R, error) {
		var err error
		var parsingValueExpressions []string
		if prependPathsContext {
			originalValueExpressions := expressions.GetValueExpressions()
			parsingValueExpressions = make([]string, 0, len(originalValueExpressions))
			for _, expr := range originalValueExpressions {
				prependedValueExpression, prependErr := parser.prependContextToValueExpressionPaths(context, expr)
				if prependErr != nil {
					err = prependErr
					break
				}
				parsingValueExpressions = append(parsingValueExpressions, prependedValueExpression)
			}
			if err != nil {
				return *new(R), err
			}
			if pc.modifiedLogging {
				pc.logModifications(originalValueExpressions, parsingValueExpressions)
			}
		} else {
			parsingValueExpressions = expressions.GetValueExpressions()
		}
		parsedValueExpressions, err := parser.ParseValueExpressions(parsingValueExpressions)
		if err != nil {
			return *new(R), err
		}
		return converter(
			pc,
			expressions,
			parsedValueExpressions,
		)
	}
}

// createStatementsParserWithConverter is a method to create the necessary parser wrapper and shadowing the K type.
func createStatementsParserWithConverter[K, R any](converter ParsedStatementsConverter[K, R], parser *Parser[K]) parserCollectionContextParserFunc[R, StatementsGetter] {
	return func(pc *ParserCollection[R], context string, statements StatementsGetter, prependPathsContext bool) (R, error) {
		var err error
		var parsingStatements []string
		if prependPathsContext {
			originalStatements := statements.GetStatements()
			parsingStatements = make([]string, 0, len(originalStatements))
			for _, cond := range originalStatements {
				prependedStatement, prependErr := parser.prependContextToStatementPaths(context, cond)
				if prependErr != nil {
					err = prependErr
					break
				}
				parsingStatements = append(parsingStatements, prependedStatement)
			}
			if err != nil {
				return *new(R), err
			}
			if pc.modifiedLogging {
				pc.logModifications(originalStatements, parsingStatements)
			}
		} else {
			parsingStatements = statements.GetStatements()
		}
		parsedStatements, err := parser.ParseStatements(parsingStatements)
		if err != nil {
			return *new(R), err
		}
		return converter(
			pc,
			statements,
			parsedStatements,
		)
	}
}

// WithConditionConverter sets the condition converter for the given context.
// The provided converter function will be used to convert parsed OTTL conditions into a common representation of type R.
// The context's OTTL parser will parse the conditions, and the converter function will transform the parsed conditions into the desired representation.
//
// Experimental: *NOTE* this API is subject to change or removal in the future.
func WithConditionConverter[K, R any](converter ParsedConditionsConverter[K, R]) ParserCollectionContextOption[K, R] {
	return func(pcp *ParserCollectionContextParser[R], parser *Parser[K]) {
		pcp.parseConditions = createConditionsParserWithConverter(converter, parser)
	}
}

// WithValueExpressionConverter sets the value expression converter for the given context.
// The provided converter function will be used to convert parsed OTTL value expressions into a common representation of type R.
// The context's OTTL parser will parse the value expressions, and the converter function will transform the parsed value expressions into the desired representation.
//
// Experimental: *NOTE* this API is subject to change or removal in the future.
func WithValueExpressionConverter[K, R any](converter ParsedValueExpressionsConverter[K, R]) ParserCollectionContextOption[K, R] {
	return func(pcp *ParserCollectionContextParser[R], parser *Parser[K]) {
		pcp.parseValueExpressions = createValueExpressionsParserWithConverter(converter, parser)
	}
}

// WithStatementConverter sets the statement converter for the given context.
// The provided converter function will be used to convert parsed OTTL statements into a common representation of type R.
// The context's OTTL parser will parse the statements, and the converter function will transform the parsed statements into the desired representation.
//
// Experimental: *NOTE* this API is subject to change or removal in the future.
func WithStatementConverter[K, R any](converter ParsedStatementsConverter[K, R]) ParserCollectionContextOption[K, R] {
	return func(pcp *ParserCollectionContextParser[R], parser *Parser[K]) {
		pcp.parseStatements = createStatementsParserWithConverter(converter, parser)
	}
}

// WithParserCollectionContext configures an ottl.Parser for the given context.
// The provided ottl.Parser must be configured to support the provided context using
// the ottl.WithPathContextNames option.
//
// Experimental: *NOTE* this API is subject to change or removal in the future.
func WithParserCollectionContext[K, R any](
	context string,
	parser *Parser[K],
	opts ...ParserCollectionContextOption[K, R],
) ParserCollectionOption[R] {
	return func(mp *ParserCollection[R]) error {
		if _, ok := parser.pathContextNames[context]; !ok {
			return fmt.Errorf(`context "%s" must be a valid "%T" path context name`, context, parser)
		}
		pcp := &ParserCollectionContextParser[R]{}
		for _, o := range opts {
			o(pcp, parser)
		}
		mp.contextParsers[context] = pcp

		for lowerContext := range parser.pathContextNames {
			if lowerContext != context {
				mp.candidatesLowerContexts[lowerContext] = append(mp.candidatesLowerContexts[lowerContext], context)
			}
		}

		mp.contextInferrerCandidates[context] = &priorityContextInferrerCandidate{
			hasEnumSymbol: func(enum *EnumSymbol) bool {
				_, err := parser.enumParser(enum)
				return err == nil
			},
			hasFunctionName: func(name string) bool {
				_, ok := parser.functions[name]
				return ok
			},
			getLowerContexts: mp.getLowerContexts,
		}
		return nil
	}
}

func (pc *ParserCollection[R]) getLowerContexts(context string) []string {
	return pc.candidatesLowerContexts[context]
}

// WithParserCollectionErrorMode has no effect on the ParserCollection, but might be used
// by the ParsedStatementsConverter functions to handle/create StatementSequence.
//
// Experimental: *NOTE* this API is subject to change or removal in the future.
func WithParserCollectionErrorMode[R any](errorMode ErrorMode) ParserCollectionOption[R] {
	return func(tp *ParserCollection[R]) error {
		tp.ErrorMode = errorMode
		return nil
	}
}

// EnableParserCollectionModifiedPathsLogging controls the modification logs.
// When enabled, it logs any modifications performed by the parsing operations,
// instructing users to rewrite the statements accordingly.
//
// Experimental: *NOTE* this API is subject to change or removal in the future.
func EnableParserCollectionModifiedPathsLogging[R any](enabled bool) ParserCollectionOption[R] {
	return func(tp *ParserCollection[R]) error {
		tp.modifiedLogging = enabled
		return nil
	}
}

type parseCollectionContextInferenceOptions struct {
	conditions []string
}

// ParserCollectionContextInferenceOption allows configuring the context inference and use
// this option with the supported parsing functions.
//
// Experimental: *NOTE* this API is subject to change or removal in the future.
type ParserCollectionContextInferenceOption func(p *parseCollectionContextInferenceOptions)

// WithContextInferenceConditions sets additional OTTL conditions to be used to enhance
// the context inference process. This is particularly useful when the statements alone are
// insufficient for determine the correct context, or when a less-specific context is desired.
//
// Experimental: *NOTE* this API is subject to change or removal in the future.
func WithContextInferenceConditions(conditions []string) ParserCollectionContextInferenceOption {
	return func(p *parseCollectionContextInferenceOptions) {
		p.conditions = conditions
	}
}

// ParseStatements parses the given statements into [R] using the configured context's ottl.Parser
// and subsequently calling the ParsedStatementsConverter function.
// The statement's context is automatically inferred from the [Path.Context] values, choosing the
// highest priority context found.
// If no contexts are present in the statements, or if the inferred value is not supported by
// the [ParserCollection], it returns an error.
// If parsing the statements fails, it returns the underlying [ottl.Parser.ParseStatements] error.
// If the provided StatementsGetter also implements ContextInferenceHintsProvider, it uses the
// additional OTTL conditions to enhance the context inference. This is particularly useful when
// the statements alone are insufficient for determine the correct context, or if an less-specific
// parser is desired.
//
// Experimental: *NOTE* this API is subject to change or removal in the future.
func (pc *ParserCollection[R]) ParseStatements(statements StatementsGetter, options ...ParserCollectionContextInferenceOption) (R, error) {
	statementsValues := statements.GetStatements()

	parseStatementsOpts := parseCollectionContextInferenceOptions{}
	for _, opt := range options {
		opt(&parseStatementsOpts)
	}

	conditionsValues := parseStatementsOpts.conditions

	var inferredContext string
	var err error
	if len(conditionsValues) > 0 {
		inferredContext, err = pc.contextInferrer.infer(statementsValues, conditionsValues, nil)
	} else {
		inferredContext, err = pc.contextInferrer.inferFromStatements(statementsValues)
	}

	if err != nil {
		return *new(R), fmt.Errorf("unable to infer a valid context (%+q) from statements %+q and conditions %+q: %w", pc.supportedContextNames(), statementsValues, conditionsValues, err)
	}

	if inferredContext == "" {
		return *new(R), fmt.Errorf("unable to infer context from statements %+q and conditions %+q, path's first segment must be a valid context name %+q, and at least one context must be capable of parsing all statements", pc.supportedContextNames(), statementsValues, conditionsValues)
	}

	_, ok := pc.contextParsers[inferredContext]
	if !ok {
		return *new(R), fmt.Errorf(`context "%s" inferred from the statements %+q and conditions %+q is not a supported context: %+q`, inferredContext, statementsValues, conditionsValues, pc.supportedContextNames())
	}

	return pc.ParseStatementsWithContext(inferredContext, statements, false)
}

// ParseStatementsWithContext parses the given statements into [R] using the configured
// context's ottl.Parser and subsequently calling the ParsedStatementsConverter function.
// Unlike ParseStatements, it uses the provided context and does not infer it
// automatically. The context value must be supported by the [ParserCollection],
// otherwise an error is returned.
// If the statement's Path does not provide their Path.Context value, the prependPathsContext
// argument should be set to true, so it rewrites the statements prepending the missing paths
// contexts.
// If parsing the statements fails, it returns the underlying [ottl.Parser.ParseStatements] error.
//
// Experimental: *NOTE* this API is subject to change or removal in the future.
func (pc *ParserCollection[R]) ParseStatementsWithContext(context string, statements StatementsGetter, prependPathsContext bool) (R, error) {
	contextParser, ok := pc.contextParsers[context]
	if !ok {
		return *new(R), fmt.Errorf(`unknown context "%s" for statements: %v`, context, statements.GetStatements())
	}
	if contextParser.parseStatements == nil {
		return *new(R), fmt.Errorf(`context "%s" has no configured converter for statements: %v`, context, statements.GetStatements())
	}
	return contextParser.parseStatements(
		pc,
		context,
		statements,
		prependPathsContext,
	)
}

// ParseConditions parses the given conditions into [R] using the configured context's ottl.Parser
// and subsequently calling the ParsedConditionsConverter function.
// The condition's context is automatically inferred from the [Path.Context] values, choosing the
// highest priority context found.
// If no contexts are present in the conditions, or if the inferred value is not supported by
// the [ParserCollection], it returns an error.
// If parsing the conditions fails, it returns the underlying [ottl.Parser.ParseConditions] error.
//
// Experimental: *NOTE* this API is subject to change or removal in the future.
func (pc *ParserCollection[R]) ParseConditions(conditions ConditionsGetter) (R, error) {
	conditionsValues := conditions.GetConditions()
	inferredContext, err := pc.contextInferrer.inferFromConditions(conditionsValues)
	if err != nil {
		return *new(R), err
	}

	if inferredContext == "" {
		return *new(R), fmt.Errorf("unable to infer context from conditions, path's first segment must be a valid context name: %+q, and at least one context must be capable of parsing all conditions: %+q", pc.supportedContextNames(), conditionsValues)
	}

	_, ok := pc.contextParsers[inferredContext]
	if !ok {
		return *new(R), fmt.Errorf(`context "%s" inferred from the conditions %+q is not a supported context: %+q`, inferredContext, conditionsValues, pc.supportedContextNames())
	}

	return pc.ParseConditionsWithContext(inferredContext, conditions, false)
}

// ParseConditionsWithContext parses the given conditions into [R] using the configured
// context's ottl.Parser and subsequently calling the ParsedConditionsConverter function.
// Unlike ParseConditions, it uses the provided context and does not infer it
// automatically. The context value must be supported by the [ParserCollection],
// otherwise an error is returned.
// If the condition's Path does not provide their Path.Context value, the prependPathsContext
// argument should be set to true, so it rewrites the conditions prepending the missing paths
// contexts.
// If parsing the conditions fails, it returns the underlying [ottl.Parser.ParseConditions] error.
//
// Experimental: *NOTE* this API is subject to change or removal in the future.
func (pc *ParserCollection[R]) ParseConditionsWithContext(context string, conditions ConditionsGetter, prependPathsContext bool) (R, error) {
	contextParser, ok := pc.contextParsers[context]
	if !ok {
		return *new(R), fmt.Errorf(`unknown context "%s" for conditions: %v`, context, conditions.GetConditions())
	}
	if contextParser.parseConditions == nil {
		return *new(R), fmt.Errorf(`context "%s" has no configured converter for conditions: %v`, context, conditions.GetConditions())
	}

	return contextParser.parseConditions(
		pc,
		context,
		conditions,
		prependPathsContext,
	)
}

// ParseValueExpressions parses the given expressions into [R] using the configured context's ottl.Parser
// and subsequently calling the ParsedValueExpressionsConverter function.
// The expression's context is automatically inferred from the [Path.Context] values, choosing the
// highest priority context found.
// If no contexts are present in the expressions, or if the inferred value is not supported by
// the [ParserCollection], it returns an error.
// If parsing the expressions fails, it returns the underlying [ottl.Parser.ParseValueExpressions] error.
//
// Experimental: *NOTE* this API is subject to change or removal in the future.
func (pc *ParserCollection[R]) ParseValueExpressions(expressions ValueExpressionsGetter, options ...ParserCollectionContextInferenceOption) (R, error) {
	expressionStrings := expressions.GetValueExpressions()

	parseStatementsOpts := parseCollectionContextInferenceOptions{}
	for _, opt := range options {
		opt(&parseStatementsOpts)
	}
	conditionsValues := parseStatementsOpts.conditions

	inferredContext, err := pc.contextInferrer.infer(nil, conditionsValues, expressionStrings)
	if err != nil {
		return *new(R), err
	}

	if inferredContext == "" {
		return *new(R), fmt.Errorf(
			"unable to infer context from expressions, path's first segment must be a valid context name: %+q, and at least one context must be capable of parsing all expressions: %+q",
			pc.supportedContextNames(),
			expressionStrings,
		)
	}

	_, ok := pc.contextParsers[inferredContext]
	if !ok {
		return *new(R), fmt.Errorf(
			`context "%s" inferred from the expressions %+q is not a supported context: %+q`,
			inferredContext,
			expressionStrings,
			pc.supportedContextNames(),
		)
	}

	return pc.ParseValueExpressionsWithContext(inferredContext, expressions, false)
}

// ParseValueExpressionsWithContext parses the given expressions into [R] using the configured
// context's ottl.Parser and subsequently calling the ParsedValueExpressionsConverter function.
// Unlike ParseValueExpressions, it uses the provided context and does not infer it
// automatically. The context value must be supported by the [ParserCollection],
// otherwise an error is returned.
// If the expression's Path does not provide their Path.Context value, the prependPathsContext
// argument should be set to true, so it rewrites the expressions prepending the missing paths
// contexts.
// If parsing the expressions fails, it returns the underlying [ottl.Parser.ParseValueExpressions] error.
//
// Experimental: *NOTE* this API is subject to change or removal in the future.
func (pc *ParserCollection[R]) ParseValueExpressionsWithContext(context string, expressions ValueExpressionsGetter, prependPathsContext bool) (R, error) {
	contextParser, ok := pc.contextParsers[context]
	if !ok {
		return *new(R), fmt.Errorf(`unknown context "%s" for value expressions: %v`, context, expressions.GetValueExpressions())
	}
	if contextParser.parseValueExpressions == nil {
		return *new(R), fmt.Errorf(`context "%s" has no configured converter for value expressions: %v`, context, expressions.GetValueExpressions())
	}

	return contextParser.parseValueExpressions(
		pc,
		context,
		expressions,
		prependPathsContext,
	)
}

func (pc *ParserCollection[R]) logModifications(originalStatements, modifiedStatements []string) {
	var fields []zap.Field
	for i, original := range originalStatements {
		if modifiedStatements[i] != original {
			statementKey := fmt.Sprintf("[%v]", i)
			fields = append(fields, zap.Dict(
				statementKey,
				zap.String("original", original),
				zap.String("modified", modifiedStatements[i])),
			)
		}
	}
	if len(fields) > 0 {
		pc.Settings.Logger.Info("one or more paths were modified to include their context prefix, please rewrite them accordingly", zap.Dict("values", fields...))
	}
}

func (pc *ParserCollection[R]) supportedContextNames() []string {
	contextsNames := make([]string, 0, len(pc.contextParsers))
	for k := range pc.contextParsers {
		contextsNames = append(contextsNames, k)
	}
	return contextsNames
}
