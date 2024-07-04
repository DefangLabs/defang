import * as pb from './io/defang/v1/fabric_pb.js';

const update = pb.ProjectUpdate.fromJsonString('{"compose":{"services":{"web":{"image":"nginx"}}}}');

interface Service {
    image: string;
}

interface Project {
    name?: string;
    services: { [name: string]: Service };
}

const project = update.compose?.toJson()?.valueOf() as Project;
console.log(project);