export namespace service {
	
	export class Config {
	    mode: string;
	    ffmpeg_dir: string;
	    ffprobe_dir: string;
	    output_dir: string;
	    timeout_sec: number;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.ffmpeg_dir = source["ffmpeg_dir"];
	        this.ffprobe_dir = source["ffprobe_dir"];
	        this.output_dir = source["output_dir"];
	        this.timeout_sec = source["timeout_sec"];
	    }
	}
	export class Job {
	    id: string;
	    type: string;
	    input: string;
	    output: string;
	    state: string;
	    progress: number;
	    speed: number;
	    error?: string;
	    error_detail?: string;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;
	
	    static createFrom(source: any = {}) {
	        return new Job(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.type = source["type"];
	        this.input = source["input"];
	        this.output = source["output"];
	        this.state = source["state"];
	        this.progress = source["progress"];
	        this.speed = source["speed"];
	        this.error = source["error"];
	        this.error_detail = source["error_detail"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
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
	export class MediaInfo {
	    path: string;
	    name: string;
	    duration_sec: number;
	    width: number;
	    height: number;
	    frame_rate: number;
	    video_codec: string;
	    audio_codec: string;
	    size_bytes: number;
	    has_audio: boolean;
	    has_video: boolean;
	
	    static createFrom(source: any = {}) {
	        return new MediaInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	        this.duration_sec = source["duration_sec"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.frame_rate = source["frame_rate"];
	        this.video_codec = source["video_codec"];
	        this.audio_codec = source["audio_codec"];
	        this.size_bytes = source["size_bytes"];
	        this.has_audio = source["has_audio"];
	        this.has_video = source["has_video"];
	    }
	}
	export class ProConvertOptions {
	    TrimStart: number;
	    TrimEnd: number;
	    Width: number;
	    Height: number;
	    Fps: number;
	    VideoCodec: string;
	    VideoBitrate: string;
	    CRF: number;
	    Preset: string;
	    AudioCodec: string;
	    AudioBitrate: string;
	    Mute: boolean;
	    ExtraArgs: string[];
	    output: string;
	
	    static createFrom(source: any = {}) {
	        return new ProConvertOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.TrimStart = source["TrimStart"];
	        this.TrimEnd = source["TrimEnd"];
	        this.Width = source["Width"];
	        this.Height = source["Height"];
	        this.Fps = source["Fps"];
	        this.VideoCodec = source["VideoCodec"];
	        this.VideoBitrate = source["VideoBitrate"];
	        this.CRF = source["CRF"];
	        this.Preset = source["Preset"];
	        this.AudioCodec = source["AudioCodec"];
	        this.AudioBitrate = source["AudioBitrate"];
	        this.Mute = source["Mute"];
	        this.ExtraArgs = source["ExtraArgs"];
	        this.output = source["output"];
	    }
	}
	export class SimpleConvertOptions {
	    preset: string;
	    quality: string;
	    trim_start: number;
	    trim_end: number;
	
	    static createFrom(source: any = {}) {
	        return new SimpleConvertOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.preset = source["preset"];
	        this.quality = source["quality"];
	        this.trim_start = source["trim_start"];
	        this.trim_end = source["trim_end"];
	    }
	}

}

