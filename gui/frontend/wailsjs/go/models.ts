export namespace main {
	
	export class ParamDiffResult {
	    oldName: string;
	    newName: string;
	    oldValue: string;
	    newValue: string;
	
	    static createFrom(source: any = {}) {
	        return new ParamDiffResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.oldName = source["oldName"];
	        this.newName = source["newName"];
	        this.oldValue = source["oldValue"];
	        this.newValue = source["newValue"];
	    }
	}
	export class ParamListEntry {
	    name: string;
	    value?: string;
	
	    static createFrom(source: any = {}) {
	        return new ParamListEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.value = source["value"];
	    }
	}
	export class ParamListResult {
	    entries: ParamListEntry[];
	
	    static createFrom(source: any = {}) {
	        return new ParamListResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.entries = this.convertValues(source["entries"], ParamListEntry);
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
	export class ParamLogEntry {
	    version: number;
	    value: string;
	    type: string;
	    isCurrent: boolean;
	    lastModified?: string;
	
	    static createFrom(source: any = {}) {
	        return new ParamLogEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.value = source["value"];
	        this.type = source["type"];
	        this.isCurrent = source["isCurrent"];
	        this.lastModified = source["lastModified"];
	    }
	}
	export class ParamLogResult {
	    name: string;
	    entries: ParamLogEntry[];
	
	    static createFrom(source: any = {}) {
	        return new ParamLogResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.entries = this.convertValues(source["entries"], ParamLogEntry);
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
	export class ParamShowResult {
	    name: string;
	    value: string;
	    version: number;
	    type: string;
	    lastModified?: string;
	
	    static createFrom(source: any = {}) {
	        return new ParamShowResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.value = source["value"];
	        this.version = source["version"];
	        this.type = source["type"];
	        this.lastModified = source["lastModified"];
	    }
	}
	export class SecretListEntry {
	    name: string;
	    value?: string;
	
	    static createFrom(source: any = {}) {
	        return new SecretListEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.value = source["value"];
	    }
	}
	export class SecretListResult {
	    entries: SecretListEntry[];
	
	    static createFrom(source: any = {}) {
	        return new SecretListResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.entries = this.convertValues(source["entries"], SecretListEntry);
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
	export class SecretLogEntry {
	    versionId: string;
	    stages: string[];
	    created?: string;
	
	    static createFrom(source: any = {}) {
	        return new SecretLogEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.versionId = source["versionId"];
	        this.stages = source["stages"];
	        this.created = source["created"];
	    }
	}
	export class SecretLogResult {
	    name: string;
	    entries: SecretLogEntry[];
	
	    static createFrom(source: any = {}) {
	        return new SecretLogResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.entries = this.convertValues(source["entries"], SecretLogEntry);
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
	export class SecretShowResult {
	    name: string;
	    arn: string;
	    versionId: string;
	    versionStage: string[];
	    value: string;
	    createdDate?: string;
	
	    static createFrom(source: any = {}) {
	        return new SecretShowResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.arn = source["arn"];
	        this.versionId = source["versionId"];
	        this.versionStage = source["versionStage"];
	        this.value = source["value"];
	        this.createdDate = source["createdDate"];
	    }
	}
	export class StagingEntry {
	    name: string;
	    operation: string;
	    value?: string;
	    stagedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new StagingEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.operation = source["operation"];
	        this.value = source["value"];
	        this.stagedAt = source["stagedAt"];
	    }
	}
	export class StagingStatusResult {
	    ssm: StagingEntry[];
	    sm: StagingEntry[];
	
	    static createFrom(source: any = {}) {
	        return new StagingStatusResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ssm = this.convertValues(source["ssm"], StagingEntry);
	        this.sm = this.convertValues(source["sm"], StagingEntry);
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

