import * as pb from './io/defang/v1/fabric_pb';
const update = pb.ProjectUpdate.fromJsonString('{"compose":{"services":{"web":{"image":"nginx"}}}}');
const project = update.compose;
console.log(project);
//# sourceMappingURL=index.js.map