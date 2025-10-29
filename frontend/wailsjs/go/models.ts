export namespace database {
	
	export class ForeignKeyRef {
	    table: string;
	    column: string;
	    schema?: string;
	
	    static createFrom(source: any = {}) {
	        return new ForeignKeyRef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.table = source["table"];
	        this.column = source["column"];
	        this.schema = source["schema"];
	    }
	}
	export class EditableColumn {
	    name: string;
	    result_name: string;
	    data_type: string;
	    editable: boolean;
	    primary_key: boolean;
	    foreign_key?: ForeignKeyRef;
	
	    static createFrom(source: any = {}) {
	        return new EditableColumn(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.result_name = source["result_name"];
	        this.data_type = source["data_type"];
	        this.editable = source["editable"];
	        this.primary_key = source["primary_key"];
	        this.foreign_key = this.convertValues(source["foreign_key"], ForeignKeyRef);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class EditableQueryMetadata {
	    enabled: boolean;
	    reason?: string;
	    schema?: string;
	    table?: string;
	    primary_keys?: string[];
	    columns?: EditableColumn[];
	    pending?: boolean;
	    job_id?: string;
	
	    static createFrom(source: any = {}) {
	        return new EditableQueryMetadata(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.reason = source["reason"];
	        this.schema = source["schema"];
	        this.table = source["table"];
	        this.primary_keys = source["primary_keys"];
	        this.columns = this.convertValues(source["columns"], EditableColumn);
	        this.pending = source["pending"];
	        this.job_id = source["job_id"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace main {
	
	export class AIMemoryMessagePayload {
	    role: string;
	    content: string;
	    timestamp: number;
	    metadata?: Record<string, any>;
	
	    static createFrom(source: any = {}) {
	        return new AIMemoryMessagePayload(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.role = source["role"];
	        this.content = source["content"];
	        this.timestamp = source["timestamp"];
	        this.metadata = source["metadata"];
	    }
	}
	export class AIMemoryRecallResult {
	    sessionId: string;
	    title: string;
	    summary?: string;
	    content: string;
	    score: number;
	
	    static createFrom(source: any = {}) {
	        return new AIMemoryRecallResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessionId = source["sessionId"];
	        this.title = source["title"];
	        this.summary = source["summary"];
	        this.content = source["content"];
	        this.score = source["score"];
	    }
	}
	export class AIMemorySessionPayload {
	    id: string;
	    title: string;
	    createdAt: number;
	    updatedAt: number;
	    summary?: string;
	    summaryTokens?: number;
	    metadata?: Record<string, any>;
	    messages: AIMemoryMessagePayload[];
	
	    static createFrom(source: any = {}) {
	        return new AIMemorySessionPayload(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	        this.summary = source["summary"];
	        this.summaryTokens = source["summaryTokens"];
	        this.metadata = source["metadata"];
	        this.messages = this.convertValues(source["messages"], AIMemoryMessagePayload);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class AIQueryAgentInsightAttachment {
	    highlights: string[];
	
	    static createFrom(source: any = {}) {
	        return new AIQueryAgentInsightAttachment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.highlights = source["highlights"];
	    }
	}
	export class AIQueryAgentReportAttachment {
	    format: string;
	    body: string;
	    title?: string;
	
	    static createFrom(source: any = {}) {
	        return new AIQueryAgentReportAttachment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.format = source["format"];
	        this.body = source["body"];
	        this.title = source["title"];
	    }
	}
	export class AIQueryAgentChartAttachment {
	    type: string;
	    xField: string;
	    yFields: string[];
	    seriesField?: string;
	    title?: string;
	    description?: string;
	    recommended: boolean;
	    previewValues?: any[];
	
	    static createFrom(source: any = {}) {
	        return new AIQueryAgentChartAttachment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.xField = source["xField"];
	        this.yFields = source["yFields"];
	        this.seriesField = source["seriesField"];
	        this.title = source["title"];
	        this.description = source["description"];
	        this.recommended = source["recommended"];
	        this.previewValues = source["previewValues"];
	    }
	}
	export class AIQueryAgentResultAttachment {
	    columns: string[];
	    rows: any[];
	    rowCount: number;
	    executionTimeMs: number;
	    limited: boolean;
	    connectionId?: string;
	
	    static createFrom(source: any = {}) {
	        return new AIQueryAgentResultAttachment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.columns = source["columns"];
	        this.rows = source["rows"];
	        this.rowCount = source["rowCount"];
	        this.executionTimeMs = source["executionTimeMs"];
	        this.limited = source["limited"];
	        this.connectionId = source["connectionId"];
	    }
	}
	export class AIQueryAgentSQLAttachment {
	    query: string;
	    explanation?: string;
	    confidence?: number;
	    connectionId?: string;
	    warnings?: string[];
	
	    static createFrom(source: any = {}) {
	        return new AIQueryAgentSQLAttachment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.query = source["query"];
	        this.explanation = source["explanation"];
	        this.confidence = source["confidence"];
	        this.connectionId = source["connectionId"];
	        this.warnings = source["warnings"];
	    }
	}
	export class AIQueryAgentAttachment {
	    type: string;
	    sql?: AIQueryAgentSQLAttachment;
	    result?: AIQueryAgentResultAttachment;
	    chart?: AIQueryAgentChartAttachment;
	    report?: AIQueryAgentReportAttachment;
	    insight?: AIQueryAgentInsightAttachment;
	    rawPayload?: Record<string, any>;
	
	    static createFrom(source: any = {}) {
	        return new AIQueryAgentAttachment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.sql = this.convertValues(source["sql"], AIQueryAgentSQLAttachment);
	        this.result = this.convertValues(source["result"], AIQueryAgentResultAttachment);
	        this.chart = this.convertValues(source["chart"], AIQueryAgentChartAttachment);
	        this.report = this.convertValues(source["report"], AIQueryAgentReportAttachment);
	        this.insight = this.convertValues(source["insight"], AIQueryAgentInsightAttachment);
	        this.rawPayload = source["rawPayload"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	export class AIQueryAgentMessage {
	    id: string;
	    agent: string;
	    role: string;
	    title?: string;
	    content: string;
	    createdAt: number;
	    attachments?: AIQueryAgentAttachment[];
	    metadata?: Record<string, any>;
	    warnings?: string[];
	    error?: string;
	    provider?: string;
	    model?: string;
	    tokensUsed?: number;
	    elapsedMs?: number;
	    context?: Record<string, Array<number>>;
	
	    static createFrom(source: any = {}) {
	        return new AIQueryAgentMessage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.agent = source["agent"];
	        this.role = source["role"];
	        this.title = source["title"];
	        this.content = source["content"];
	        this.createdAt = source["createdAt"];
	        this.attachments = this.convertValues(source["attachments"], AIQueryAgentAttachment);
	        this.metadata = source["metadata"];
	        this.warnings = source["warnings"];
	        this.error = source["error"];
	        this.provider = source["provider"];
	        this.model = source["model"];
	        this.tokensUsed = source["tokensUsed"];
	        this.elapsedMs = source["elapsedMs"];
	        this.context = source["context"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class AIQueryAgentRequest {
	    sessionId: string;
	    message: string;
	    provider: string;
	    model: string;
	    connectionId?: string;
	    connectionIds?: string[];
	    schemaContext?: string;
	    context?: string;
	    history?: AIMemoryMessagePayload[];
	    systemPrompt?: string;
	    temperature?: number;
	    maxTokens?: number;
	    maxRows?: number;
	
	    static createFrom(source: any = {}) {
	        return new AIQueryAgentRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessionId = source["sessionId"];
	        this.message = source["message"];
	        this.provider = source["provider"];
	        this.model = source["model"];
	        this.connectionId = source["connectionId"];
	        this.connectionIds = source["connectionIds"];
	        this.schemaContext = source["schemaContext"];
	        this.context = source["context"];
	        this.history = this.convertValues(source["history"], AIMemoryMessagePayload);
	        this.systemPrompt = source["systemPrompt"];
	        this.temperature = source["temperature"];
	        this.maxTokens = source["maxTokens"];
	        this.maxRows = source["maxRows"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class AIQueryAgentResponse {
	    sessionId: string;
	    turnId: string;
	    provider: string;
	    model: string;
	    messages: AIQueryAgentMessage[];
	    error?: string;
	    durationMs: number;
	    metadata?: Record<string, any>;
	    executedSql?: string;
	
	    static createFrom(source: any = {}) {
	        return new AIQueryAgentResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sessionId = source["sessionId"];
	        this.turnId = source["turnId"];
	        this.provider = source["provider"];
	        this.model = source["model"];
	        this.messages = this.convertValues(source["messages"], AIQueryAgentMessage);
	        this.error = source["error"];
	        this.durationMs = source["durationMs"];
	        this.metadata = source["metadata"];
	        this.executedSql = source["executedSql"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	export class AITestResponse {
	    success: boolean;
	    message: string;
	    error?: string;
	    metadata?: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new AITestResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.message = source["message"];
	        this.error = source["error"];
	        this.metadata = source["metadata"];
	    }
	}
	export class AlternativeQuery {
	    sql: string;
	    confidence: number;
	    description: string;
	
	    static createFrom(source: any = {}) {
	        return new AlternativeQuery(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sql = source["sql"];
	        this.confidence = source["confidence"];
	        this.description = source["description"];
	    }
	}
	export class ColumnInfo {
	    name: string;
	    data_type: string;
	    nullable: boolean;
	    default_value?: string;
	    primary_key: boolean;
	    unique: boolean;
	    indexed: boolean;
	    comment: string;
	    ordinal_position: number;
	    character_maximum_length?: number;
	    numeric_precision?: number;
	    numeric_scale?: number;
	    metadata: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new ColumnInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.data_type = source["data_type"];
	        this.nullable = source["nullable"];
	        this.default_value = source["default_value"];
	        this.primary_key = source["primary_key"];
	        this.unique = source["unique"];
	        this.indexed = source["indexed"];
	        this.comment = source["comment"];
	        this.ordinal_position = source["ordinal_position"];
	        this.character_maximum_length = source["character_maximum_length"];
	        this.numeric_precision = source["numeric_precision"];
	        this.numeric_scale = source["numeric_scale"];
	        this.metadata = source["metadata"];
	    }
	}
	export class ConflictingTable {
	    connectionId: string;
	    tableName: string;
	    schema: string;
	
	    static createFrom(source: any = {}) {
	        return new ConflictingTable(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connectionId = source["connectionId"];
	        this.tableName = source["tableName"];
	        this.schema = source["schema"];
	    }
	}
	export class SchemaConflict {
	    tableName: string;
	    connections: ConflictingTable[];
	    resolution: string;
	
	    static createFrom(source: any = {}) {
	        return new SchemaConflict(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tableName = source["tableName"];
	        this.connections = this.convertValues(source["connections"], ConflictingTable);
	        this.resolution = source["resolution"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TableInfo {
	    schema: string;
	    name: string;
	    type: string;
	    comment: string;
	    rowCount: number;
	    sizeBytes: number;
	
	    static createFrom(source: any = {}) {
	        return new TableInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.schema = source["schema"];
	        this.name = source["name"];
	        this.type = source["type"];
	        this.comment = source["comment"];
	        this.rowCount = source["rowCount"];
	        this.sizeBytes = source["sizeBytes"];
	    }
	}
	export class ConnectionSchema {
	    connectionId: string;
	    name: string;
	    type: string;
	    schemas: string[];
	    tables: TableInfo[];
	
	    static createFrom(source: any = {}) {
	        return new ConnectionSchema(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connectionId = source["connectionId"];
	        this.name = source["name"];
	        this.type = source["type"];
	        this.schemas = source["schemas"];
	        this.tables = this.convertValues(source["tables"], TableInfo);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CombinedSchema {
	    connections: Record<string, ConnectionSchema>;
	    conflicts: SchemaConflict[];
	
	    static createFrom(source: any = {}) {
	        return new CombinedSchema(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connections = this.convertValues(source["connections"], ConnectionSchema, true);
	        this.conflicts = this.convertValues(source["conflicts"], SchemaConflict);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class ConnectionInfo {
	    id: string;
	    type: string;
	    host: string;
	    port: number;
	    database: string;
	    username: string;
	    active: boolean;
	    createdAt: string;
	
	    static createFrom(source: any = {}) {
	        return new ConnectionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.type = source["type"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.database = source["database"];
	        this.username = source["username"];
	        this.active = source["active"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class ConnectionRequest {
	    id?: string;
	    name?: string;
	    type: string;
	    host: string;
	    port: number;
	    database: string;
	    username: string;
	    password: string;
	    sslMode?: string;
	    connectionTimeout?: number;
	    parameters?: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new ConnectionRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.type = source["type"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.database = source["database"];
	        this.username = source["username"];
	        this.password = source["password"];
	        this.sslMode = source["sslMode"];
	        this.connectionTimeout = source["connectionTimeout"];
	        this.parameters = source["parameters"];
	    }
	}
	
	export class EditableMetadataJobResponse {
	    id: string;
	    connectionId: string;
	    status: string;
	    metadata?: database.EditableQueryMetadata;
	    error?: string;
	    createdAt: string;
	    completedAt?: string;
	
	    static createFrom(source: any = {}) {
	        return new EditableMetadataJobResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.connectionId = source["connectionId"];
	        this.status = source["status"];
	        this.metadata = this.convertValues(source["metadata"], database.EditableQueryMetadata);
	        this.error = source["error"];
	        this.createdAt = source["createdAt"];
	        this.completedAt = source["completedAt"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class FixSQLRequest {
	    query: string;
	    error: string;
	    connectionId: string;
	    provider?: string;
	    model?: string;
	    maxTokens?: number;
	    temperature?: number;
	    context?: string;
	
	    static createFrom(source: any = {}) {
	        return new FixSQLRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.query = source["query"];
	        this.error = source["error"];
	        this.connectionId = source["connectionId"];
	        this.provider = source["provider"];
	        this.model = source["model"];
	        this.maxTokens = source["maxTokens"];
	        this.temperature = source["temperature"];
	        this.context = source["context"];
	    }
	}
	export class FixedSQLResponse {
	    sql: string;
	    explanation: string;
	    changes: string[];
	
	    static createFrom(source: any = {}) {
	        return new FixedSQLResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sql = source["sql"];
	        this.explanation = source["explanation"];
	        this.changes = source["changes"];
	    }
	}
	export class ForeignKeyInfo {
	    name: string;
	    columns: string[];
	    referenced_table: string;
	    referenced_schema: string;
	    referenced_columns: string[];
	    on_delete: string;
	    on_update: string;
	
	    static createFrom(source: any = {}) {
	        return new ForeignKeyInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.columns = source["columns"];
	        this.referenced_table = source["referenced_table"];
	        this.referenced_schema = source["referenced_schema"];
	        this.referenced_columns = source["referenced_columns"];
	        this.on_delete = source["on_delete"];
	        this.on_update = source["on_update"];
	    }
	}
	export class GeneratedSQLResponse {
	    sql: string;
	    confidence: number;
	    explanation: string;
	    warnings?: string[];
	    alternatives?: AlternativeQuery[];
	
	    static createFrom(source: any = {}) {
	        return new GeneratedSQLResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sql = source["sql"];
	        this.confidence = source["confidence"];
	        this.explanation = source["explanation"];
	        this.warnings = source["warnings"];
	        this.alternatives = this.convertValues(source["alternatives"], AlternativeQuery);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class GenericChatRequest {
	    prompt: string;
	    context?: string;
	    system?: string;
	    provider?: string;
	    model?: string;
	    maxTokens?: number;
	    temperature?: number;
	    metadata?: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new GenericChatRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.prompt = source["prompt"];
	        this.context = source["context"];
	        this.system = source["system"];
	        this.provider = source["provider"];
	        this.model = source["model"];
	        this.maxTokens = source["maxTokens"];
	        this.temperature = source["temperature"];
	        this.metadata = source["metadata"];
	    }
	}
	export class GenericChatResponse {
	    content: string;
	    provider: string;
	    model: string;
	    tokensUsed?: number;
	    metadata?: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new GenericChatResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.content = source["content"];
	        this.provider = source["provider"];
	        this.model = source["model"];
	        this.tokensUsed = source["tokensUsed"];
	        this.metadata = source["metadata"];
	    }
	}
	export class HealthStatus {
	    status: string;
	    message: string;
	    timestamp: string;
	    response_time: number;
	    metrics: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new HealthStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.message = source["message"];
	        this.timestamp = source["timestamp"];
	        this.response_time = source["response_time"];
	        this.metrics = source["metrics"];
	    }
	}
	export class IndexInfo {
	    name: string;
	    columns: string[];
	    unique: boolean;
	    primary: boolean;
	    type: string;
	    method: string;
	    metadata: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new IndexInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.columns = source["columns"];
	        this.unique = source["unique"];
	        this.primary = source["primary"];
	        this.type = source["type"];
	        this.method = source["method"];
	        this.metadata = source["metadata"];
	    }
	}
	export class MultiQueryRequest {
	    query: string;
	    limit?: number;
	    timeout?: number;
	    strategy?: string;
	    options?: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new MultiQueryRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.query = source["query"];
	        this.limit = source["limit"];
	        this.timeout = source["timeout"];
	        this.strategy = source["strategy"];
	        this.options = source["options"];
	    }
	}
	export class MultiQueryResponse {
	    columns: string[];
	    rows: any[][];
	    rowCount: number;
	    duration: string;
	    connectionsUsed: string[];
	    strategy: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new MultiQueryResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.columns = source["columns"];
	        this.rows = source["rows"];
	        this.rowCount = source["rowCount"];
	        this.duration = source["duration"];
	        this.connectionsUsed = source["connectionsUsed"];
	        this.strategy = source["strategy"];
	        this.error = source["error"];
	    }
	}
	export class NLQueryRequest {
	    prompt: string;
	    connectionId: string;
	    context?: string;
	    provider?: string;
	    model?: string;
	    maxTokens?: number;
	    temperature?: number;
	
	    static createFrom(source: any = {}) {
	        return new NLQueryRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.prompt = source["prompt"];
	        this.connectionId = source["connectionId"];
	        this.context = source["context"];
	        this.provider = source["provider"];
	        this.model = source["model"];
	        this.maxTokens = source["maxTokens"];
	        this.temperature = source["temperature"];
	    }
	}
	export class Suggestion {
	    text: string;
	    type: string;
	    detail?: string;
	    confidence?: number;
	    description?: string;
	    sql?: string;
	
	    static createFrom(source: any = {}) {
	        return new Suggestion(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	        this.type = source["type"];
	        this.detail = source["detail"];
	        this.confidence = source["confidence"];
	        this.description = source["description"];
	        this.sql = source["sql"];
	    }
	}
	export class OptimizationResponse {
	    sql: string;
	    estimatedSpeedup: string;
	    explanation: string;
	    suggestions: Suggestion[];
	
	    static createFrom(source: any = {}) {
	        return new OptimizationResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sql = source["sql"];
	        this.estimatedSpeedup = source["estimatedSpeedup"];
	        this.explanation = source["explanation"];
	        this.suggestions = this.convertValues(source["suggestions"], Suggestion);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ProviderConfig {
	    provider: string;
	    apiKey?: string;
	    endpoint?: string;
	    model?: string;
	    options?: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new ProviderConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.apiKey = source["apiKey"];
	        this.endpoint = source["endpoint"];
	        this.model = source["model"];
	        this.options = source["options"];
	    }
	}
	export class ProviderStatus {
	    name: string;
	    available: boolean;
	    error?: string;
	    model?: string;
	
	    static createFrom(source: any = {}) {
	        return new ProviderStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.available = source["available"];
	        this.error = source["error"];
	        this.model = source["model"];
	    }
	}
	export class QueryRequest {
	    connectionId: string;
	    query: string;
	    limit?: number;
	    timeout?: number;
	
	    static createFrom(source: any = {}) {
	        return new QueryRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connectionId = source["connectionId"];
	        this.query = source["query"];
	        this.limit = source["limit"];
	        this.timeout = source["timeout"];
	    }
	}
	export class QueryResponse {
	    columns: string[];
	    rows: any[][];
	    rowCount: number;
	    affected: number;
	    duration: string;
	    error?: string;
	    editable?: database.EditableQueryMetadata;
	
	    static createFrom(source: any = {}) {
	        return new QueryResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.columns = source["columns"];
	        this.rows = source["rows"];
	        this.rowCount = source["rowCount"];
	        this.affected = source["affected"];
	        this.duration = source["duration"];
	        this.error = source["error"];
	        this.editable = this.convertValues(source["editable"], database.EditableQueryMetadata);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class QueryRowUpdateRequest {
	    connectionId: string;
	    query: string;
	    columns: string[];
	    schema?: string;
	    table?: string;
	    primaryKey: Record<string, any>;
	    values: Record<string, any>;
	
	    static createFrom(source: any = {}) {
	        return new QueryRowUpdateRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connectionId = source["connectionId"];
	        this.query = source["query"];
	        this.columns = source["columns"];
	        this.schema = source["schema"];
	        this.table = source["table"];
	        this.primaryKey = source["primaryKey"];
	        this.values = source["values"];
	    }
	}
	export class QueryRowUpdateResponse {
	    success: boolean;
	    message?: string;
	
	    static createFrom(source: any = {}) {
	        return new QueryRowUpdateResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.message = source["message"];
	    }
	}
	export class ReadOnlyQueryResult {
	    columns: string[];
	    rows: any[];
	    rowCount: number;
	    executionTimeMs: number;
	    limited: boolean;
	    connectionId: string;
	
	    static createFrom(source: any = {}) {
	        return new ReadOnlyQueryResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.columns = source["columns"];
	        this.rows = source["rows"];
	        this.rowCount = source["rowCount"];
	        this.executionTimeMs = source["executionTimeMs"];
	        this.limited = source["limited"];
	        this.connectionId = source["connectionId"];
	    }
	}
	export class ResultData {
	    columns: string[];
	    rows: any[][];
	    rowCount: number;
	
	    static createFrom(source: any = {}) {
	        return new ResultData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.columns = source["columns"];
	        this.rows = source["rows"];
	        this.rowCount = source["rowCount"];
	    }
	}
	
	
	export class SyntheticViewSummary {
	    id: string;
	    name: string;
	    description: string;
	    version: string;
	    createdAt: string;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new SyntheticViewSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.version = source["version"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	
	export class TableStructure {
	    table: TableInfo;
	    columns: ColumnInfo[];
	    indexes: IndexInfo[];
	    foreign_keys: ForeignKeyInfo[];
	    triggers: string[];
	    statistics: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new TableStructure(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.table = this.convertValues(source["table"], TableInfo);
	        this.columns = this.convertValues(source["columns"], ColumnInfo);
	        this.indexes = this.convertValues(source["indexes"], IndexInfo);
	        this.foreign_keys = this.convertValues(source["foreign_keys"], ForeignKeyInfo);
	        this.triggers = source["triggers"];
	        this.statistics = source["statistics"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ValidationResult {
	    valid: boolean;
	    errors?: string[];
	    requiredConnections?: string[];
	    tables?: string[];
	    estimatedStrategy?: string;
	
	    static createFrom(source: any = {}) {
	        return new ValidationResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.valid = source["valid"];
	        this.errors = source["errors"];
	        this.requiredConnections = source["requiredConnections"];
	        this.tables = source["tables"];
	        this.estimatedStrategy = source["estimatedStrategy"];
	    }
	}
	export class VizSuggestion {
	    chartType: string;
	    title: string;
	    description: string;
	    config: Record<string, string>;
	    confidence: number;
	
	    static createFrom(source: any = {}) {
	        return new VizSuggestion(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.chartType = source["chartType"];
	        this.title = source["title"];
	        this.description = source["description"];
	        this.config = source["config"];
	        this.confidence = source["confidence"];
	    }
	}

}

export namespace services {
	
	export class FileInfo {
	    name: string;
	    path: string;
	    size: number;
	    modTime: string;
	    isDirectory: boolean;
	    extension: string;
	    permissions: string;
	
	    static createFrom(source: any = {}) {
	        return new FileInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.size = source["size"];
	        this.modTime = source["modTime"];
	        this.isDirectory = source["isDirectory"];
	        this.extension = source["extension"];
	        this.permissions = source["permissions"];
	    }
	}
	export class KeyboardAction {
	    key: string;
	    description: string;
	    category: string;
	    handler: string;
	
	    static createFrom(source: any = {}) {
	        return new KeyboardAction(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.description = source["description"];
	        this.category = source["category"];
	        this.handler = source["handler"];
	    }
	}
	export class KeyboardEvent {
	    key: string;
	    ctrlKey: boolean;
	    altKey: boolean;
	    shiftKey: boolean;
	    metaKey: boolean;
	    timestamp: number;
	
	    static createFrom(source: any = {}) {
	        return new KeyboardEvent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.ctrlKey = source["ctrlKey"];
	        this.altKey = source["altKey"];
	        this.shiftKey = source["shiftKey"];
	        this.metaKey = source["metaKey"];
	        this.timestamp = source["timestamp"];
	    }
	}
	export class RecentFile {
	    path: string;
	    name: string;
	    lastOpened: string;
	    size: number;
	
	    static createFrom(source: any = {}) {
	        return new RecentFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	        this.lastOpened = source["lastOpened"];
	        this.size = source["size"];
	    }
	}

}

export namespace storage {
	
	export class ColumnDefinition {
	    name: string;
	    type: string;
	
	    static createFrom(source: any = {}) {
	        return new ColumnDefinition(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	    }
	}
	export class SourceDefinition {
	    connectionIdOrName: string;
	    schema: string;
	    table: string;
	
	    static createFrom(source: any = {}) {
	        return new SourceDefinition(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connectionIdOrName = source["connectionIdOrName"];
	        this.schema = source["schema"];
	        this.table = source["table"];
	    }
	}
	export class ViewOptions {
	    rowLimitDefault: number;
	    materializeTemp: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ViewOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rowLimitDefault = source["rowLimitDefault"];
	        this.materializeTemp = source["materializeTemp"];
	    }
	}
	export class ViewDefinition {
	    id: string;
	    name: string;
	    description: string;
	    version: string;
	    columns: ColumnDefinition[];
	    ir: Record<string, any>;
	    sources: SourceDefinition[];
	    compiledDuckDBSQL: string;
	    options: ViewOptions;
	
	    static createFrom(source: any = {}) {
	        return new ViewDefinition(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.version = source["version"];
	        this.columns = this.convertValues(source["columns"], ColumnDefinition);
	        this.ir = source["ir"];
	        this.sources = this.convertValues(source["sources"], SourceDefinition);
	        this.compiledDuckDBSQL = source["compiledDuckDBSQL"];
	        this.options = this.convertValues(source["options"], ViewOptions);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

