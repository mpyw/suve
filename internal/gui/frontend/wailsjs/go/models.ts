export namespace gui {
	
	export class AWSIdentityResult {
	    accountId: string;
	    region: string;
	    profile: string;
	
	    static createFrom(source: any = {}) {
	        return new AWSIdentityResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.accountId = source["accountId"];
	        this.region = source["region"];
	        this.profile = source["profile"];
	    }
	}
	export class ParamDeleteResult {
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new ParamDeleteResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	    }
	}
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
	    type: string;
	    value?: string;
	
	    static createFrom(source: any = {}) {
	        return new ParamListEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.value = source["value"];
	    }
	}
	export class ParamListResult {
	    entries: ParamListEntry[];
	    nextToken?: string;
	
	    static createFrom(source: any = {}) {
	        return new ParamListResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.entries = this.convertValues(source["entries"], ParamListEntry);
	        this.nextToken = source["nextToken"];
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
	export class ParamSetResult {
	    name: string;
	    version: number;
	    isCreated: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ParamSetResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.version = source["version"];
	        this.isCreated = source["isCreated"];
	    }
	}
	export class ParamShowTag {
	    key: string;
	    value: string;
	
	    static createFrom(source: any = {}) {
	        return new ParamShowTag(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.value = source["value"];
	    }
	}
	export class ParamShowResult {
	    name: string;
	    value: string;
	    version: number;
	    type: string;
	    description?: string;
	    lastModified?: string;
	    tags: ParamShowTag[];
	
	    static createFrom(source: any = {}) {
	        return new ParamShowResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.value = source["value"];
	        this.version = source["version"];
	        this.type = source["type"];
	        this.description = source["description"];
	        this.lastModified = source["lastModified"];
	        this.tags = this.convertValues(source["tags"], ParamShowTag);
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
	
	export class SecretCreateResult {
	    name: string;
	    versionId: string;
	    arn: string;
	
	    static createFrom(source: any = {}) {
	        return new SecretCreateResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.versionId = source["versionId"];
	        this.arn = source["arn"];
	    }
	}
	export class SecretDeleteResult {
	    name: string;
	    deletionDate?: string;
	    arn: string;
	
	    static createFrom(source: any = {}) {
	        return new SecretDeleteResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.deletionDate = source["deletionDate"];
	        this.arn = source["arn"];
	    }
	}
	export class SecretDiffResult {
	    oldName: string;
	    oldVersionId: string;
	    oldValue: string;
	    newName: string;
	    newVersionId: string;
	    newValue: string;
	
	    static createFrom(source: any = {}) {
	        return new SecretDiffResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.oldName = source["oldName"];
	        this.oldVersionId = source["oldVersionId"];
	        this.oldValue = source["oldValue"];
	        this.newName = source["newName"];
	        this.newVersionId = source["newVersionId"];
	        this.newValue = source["newValue"];
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
	    nextToken?: string;
	
	    static createFrom(source: any = {}) {
	        return new SecretListResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.entries = this.convertValues(source["entries"], SecretListEntry);
	        this.nextToken = source["nextToken"];
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
	    value: string;
	    isCurrent: boolean;
	    created?: string;
	
	    static createFrom(source: any = {}) {
	        return new SecretLogEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.versionId = source["versionId"];
	        this.stages = source["stages"];
	        this.value = source["value"];
	        this.isCurrent = source["isCurrent"];
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
	export class SecretRestoreResult {
	    name: string;
	    arn: string;
	
	    static createFrom(source: any = {}) {
	        return new SecretRestoreResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.arn = source["arn"];
	    }
	}
	export class SecretShowTag {
	    key: string;
	    value: string;
	
	    static createFrom(source: any = {}) {
	        return new SecretShowTag(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.value = source["value"];
	    }
	}
	export class SecretShowResult {
	    name: string;
	    arn: string;
	    versionId: string;
	    versionStage: string[];
	    value: string;
	    description?: string;
	    createdDate?: string;
	    tags: SecretShowTag[];
	
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
	        this.description = source["description"];
	        this.createdDate = source["createdDate"];
	        this.tags = this.convertValues(source["tags"], SecretShowTag);
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
	
	export class SecretUpdateResult {
	    name: string;
	    versionId: string;
	    arn: string;
	
	    static createFrom(source: any = {}) {
	        return new SecretUpdateResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.versionId = source["versionId"];
	        this.arn = source["arn"];
	    }
	}
	export class StagingAddResult {
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new StagingAddResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	    }
	}
	export class StagingAddTagResult {
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new StagingAddTagResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	    }
	}
	export class StagingApplyEntryResult {
	    name: string;
	    status: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new StagingApplyEntryResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.status = source["status"];
	        this.error = source["error"];
	    }
	}
	export class StagingApplyTagResult {
	    name: string;
	    addTags?: Record<string, string>;
	    removeTags?: string[];
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new StagingApplyTagResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.addTags = source["addTags"];
	        this.removeTags = source["removeTags"];
	        this.error = source["error"];
	    }
	}
	export class StagingApplyResult {
	    serviceName: string;
	    entryResults: StagingApplyEntryResult[];
	    tagResults: StagingApplyTagResult[];
	    conflicts?: string[];
	    entrySucceeded: number;
	    entryFailed: number;
	    tagSucceeded: number;
	    tagFailed: number;
	
	    static createFrom(source: any = {}) {
	        return new StagingApplyResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.serviceName = source["serviceName"];
	        this.entryResults = this.convertValues(source["entryResults"], StagingApplyEntryResult);
	        this.tagResults = this.convertValues(source["tagResults"], StagingApplyTagResult);
	        this.conflicts = source["conflicts"];
	        this.entrySucceeded = source["entrySucceeded"];
	        this.entryFailed = source["entryFailed"];
	        this.tagSucceeded = source["tagSucceeded"];
	        this.tagFailed = source["tagFailed"];
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
	
	export class StagingCancelAddTagResult {
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new StagingCancelAddTagResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	    }
	}
	export class StagingCancelRemoveTagResult {
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new StagingCancelRemoveTagResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	    }
	}
	export class StagingCheckStatusResult {
	    hasEntry: boolean;
	    hasTags: boolean;
	
	    static createFrom(source: any = {}) {
	        return new StagingCheckStatusResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hasEntry = source["hasEntry"];
	        this.hasTags = source["hasTags"];
	    }
	}
	export class StagingDeleteResult {
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new StagingDeleteResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	    }
	}
	export class StagingDiffEntry {
	    name: string;
	    type: string;
	    operation?: string;
	    awsValue?: string;
	    awsIdentifier?: string;
	    stagedValue?: string;
	    description?: string;
	    warning?: string;
	
	    static createFrom(source: any = {}) {
	        return new StagingDiffEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.operation = source["operation"];
	        this.awsValue = source["awsValue"];
	        this.awsIdentifier = source["awsIdentifier"];
	        this.stagedValue = source["stagedValue"];
	        this.description = source["description"];
	        this.warning = source["warning"];
	    }
	}
	export class StagingDiffTagEntry {
	    name: string;
	    addTags?: Record<string, string>;
	    removeTags?: string[];
	
	    static createFrom(source: any = {}) {
	        return new StagingDiffTagEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.addTags = source["addTags"];
	        this.removeTags = source["removeTags"];
	    }
	}
	export class StagingDiffResult {
	    itemName: string;
	    entries: StagingDiffEntry[];
	    tagEntries: StagingDiffTagEntry[];
	
	    static createFrom(source: any = {}) {
	        return new StagingDiffResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.itemName = source["itemName"];
	        this.entries = this.convertValues(source["entries"], StagingDiffEntry);
	        this.tagEntries = this.convertValues(source["tagEntries"], StagingDiffTagEntry);
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
	
	export class StagingEditResult {
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new StagingEditResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
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
	export class StagingRemoveTagResult {
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new StagingRemoveTagResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	    }
	}
	export class StagingResetResult {
	    type: string;
	    name?: string;
	    count?: number;
	    serviceName: string;
	
	    static createFrom(source: any = {}) {
	        return new StagingResetResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.name = source["name"];
	        this.count = source["count"];
	        this.serviceName = source["serviceName"];
	    }
	}
	export class StagingTagEntry {
	    name: string;
	    addTags?: Record<string, string>;
	    removeTags?: string[];
	    stagedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new StagingTagEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.addTags = source["addTags"];
	        this.removeTags = source["removeTags"];
	        this.stagedAt = source["stagedAt"];
	    }
	}
	export class StagingStatusResult {
	    param: StagingEntry[];
	    secret: StagingEntry[];
	    paramTags: StagingTagEntry[];
	    secretTags: StagingTagEntry[];
	
	    static createFrom(source: any = {}) {
	        return new StagingStatusResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.param = this.convertValues(source["param"], StagingEntry);
	        this.secret = this.convertValues(source["secret"], StagingEntry);
	        this.paramTags = this.convertValues(source["paramTags"], StagingTagEntry);
	        this.secretTags = this.convertValues(source["secretTags"], StagingTagEntry);
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
	
	export class StagingUnstageResult {
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new StagingUnstageResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	    }
	}

}

