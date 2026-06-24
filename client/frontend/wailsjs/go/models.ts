export namespace backend {
	
	export class AppSettings {
	    unattended_password: string;
	    relay_url: string;
	
	    static createFrom(source: any = {}) {
	        return new AppSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.unattended_password = source["unattended_password"];
	        this.relay_url = source["relay_url"];
	    }
	}
	export class LocalAgentInfo {
	    id: string;
	    password: string;
	
	    static createFrom(source: any = {}) {
	        return new LocalAgentInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.password = source["password"];
	    }
	}
	export class LogEntry {
	    time: string;
	    level: string;
	    message: string;
	    raw?: string;
	
	    static createFrom(source: any = {}) {
	        return new LogEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.time = source["time"];
	        this.level = source["level"];
	        this.message = source["message"];
	        this.raw = source["raw"];
	    }
	}
	export class RecentConnection {
	    agent_id: string;
	    label: string;
	    last_used: string;
	
	    static createFrom(source: any = {}) {
	        return new RecentConnection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.agent_id = source["agent_id"];
	        this.label = source["label"];
	        this.last_used = source["last_used"];
	    }
	}
	export class SessionInfo {
	    agent_id: string;
	    hostname: string;
	    os: string;
	    state: string;
	    latency_ms: number;
	    bitrate_kbps: number;
	    fps: number;
	    started_at: string;
	
	    static createFrom(source: any = {}) {
	        return new SessionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.agent_id = source["agent_id"];
	        this.hostname = source["hostname"];
	        this.os = source["os"];
	        this.state = source["state"];
	        this.latency_ms = source["latency_ms"];
	        this.bitrate_kbps = source["bitrate_kbps"];
	        this.fps = source["fps"];
	        this.started_at = source["started_at"];
	    }
	}

}

