export namespace main {
	
	export class Account {
	    id: string;
	    email: string;
	    label: string;
	    status: string;
	    addedAt: string;
	    provider: string;
	    accessToken?: string;
	    refreshToken: string;
	    expiresAt?: string;
	    authMethod?: string;
	    clientId?: string;
	    clientSecret?: string;
	    clientIdHash?: string;
	    region?: string;
	    profileArn?: string;
	    userId?: string;
	    usageData?: Record<string, any>;
	    machineId?: string;
	
	    static createFrom(source: any = {}) {
	        return new Account(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.email = source["email"];
	        this.label = source["label"];
	        this.status = source["status"];
	        this.addedAt = source["addedAt"];
	        this.provider = source["provider"];
	        this.accessToken = source["accessToken"];
	        this.refreshToken = source["refreshToken"];
	        this.expiresAt = source["expiresAt"];
	        this.authMethod = source["authMethod"];
	        this.clientId = source["clientId"];
	        this.clientSecret = source["clientSecret"];
	        this.clientIdHash = source["clientIdHash"];
	        this.region = source["region"];
	        this.profileArn = source["profileArn"];
	        this.userId = source["userId"];
	        this.usageData = source["usageData"];
	        this.machineId = source["machineId"];
	    }
	}
	export class ActivationData {
	    code: string;
	    activated: boolean;
	    machineId: string;
	    time: string;
	
	    static createFrom(source: any = {}) {
	        return new ActivationData(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.code = source["code"];
	        this.activated = source["activated"];
	        this.machineId = source["machineId"];
	        this.time = source["time"];
	    }
	}
	export class CredentialsInfo {
	    exists: boolean;
	    source: string;
	    access_token: string;
	    refresh_token: string;
	    expires_at: string;
	    auth_method: string;
	    client_id: string;
	    client_secret: string;
	    expired: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CredentialsInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.exists = source["exists"];
	        this.source = source["source"];
	        this.access_token = source["access_token"];
	        this.refresh_token = source["refresh_token"];
	        this.expires_at = source["expires_at"];
	        this.auth_method = source["auth_method"];
	        this.client_id = source["client_id"];
	        this.client_secret = source["client_secret"];
	        this.expired = source["expired"];
	    }
	}
	export class KeychainSource {
	    source: string;
	    expires_at: string;
	    has_device: boolean;
	    provider: string;
	    expired: boolean;
	
	    static createFrom(source: any = {}) {
	        return new KeychainSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.source = source["source"];
	        this.expires_at = source["expires_at"];
	        this.has_device = source["has_device"];
	        this.provider = source["provider"];
	        this.expired = source["expired"];
	    }
	}
	export class PromptTemplate {
	    id: string;
	    name: string;
	    content: string;
	    builtin: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PromptTemplate(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.content = source["content"];
	        this.builtin = source["builtin"];
	    }
	}
	export class ProxyConfig {
	    host: string;
	    port: number;
	    apiKey: string;
	    region: string;
	    tlsBackend: string;
	    adminApiKey?: string;
	
	    static createFrom(source: any = {}) {
	        return new ProxyConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.port = source["port"];
	        this.apiKey = source["apiKey"];
	        this.region = source["region"];
	        this.tlsBackend = source["tlsBackend"];
	        this.adminApiKey = source["adminApiKey"];
	    }
	}
	export class ServerSyncConfig {
	    serverUrl: string;
	    activationCode: string;
	
	    static createFrom(source: any = {}) {
	        return new ServerSyncConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serverUrl = source["serverUrl"];
	        this.activationCode = source["activationCode"];
	    }
	}
	export class StatusInfo {
	    running: boolean;
	    has_credentials: boolean;
	    config: ProxyConfig;
	
	    static createFrom(source: any = {}) {
	        return new StatusInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.has_credentials = source["has_credentials"];
	        this.config = this.convertValues(source["config"], ProxyConfig);
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
	export class TunnelConfig {
	    enabled: boolean;
	    tunnelMode: string;
	    serverAddr: string;
	    serverPort: number;
	    token: string;
	    proxyName: string;
	    customDomain: string;
	    remotePort?: number;
	    proxyType: string;
	    vhostHTTPPort?: number;
	    externalUrl?: string;
	
	    static createFrom(source: any = {}) {
	        return new TunnelConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.tunnelMode = source["tunnelMode"];
	        this.serverAddr = source["serverAddr"];
	        this.serverPort = source["serverPort"];
	        this.token = source["token"];
	        this.proxyName = source["proxyName"];
	        this.customDomain = source["customDomain"];
	        this.remotePort = source["remotePort"];
	        this.proxyType = source["proxyType"];
	        this.vhostHTTPPort = source["vhostHTTPPort"];
	        this.externalUrl = source["externalUrl"];
	    }
	}
	export class TunnelStatus {
	    running: boolean;
	    publicUrl: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new TunnelStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.publicUrl = source["publicUrl"];
	        this.error = source["error"];
	    }
	}

}

