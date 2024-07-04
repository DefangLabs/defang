var _a, _b;
import * as pb from './io/defang/v1/fabric_pb.js';
const update = pb.ProjectUpdate.fromJsonString('{"compose":{"services":{"web":{"image":"nginx"}}}}');
const project = (_b = (_a = update.compose) === null || _a === void 0 ? void 0 : _a.toJson()) === null || _b === void 0 ? void 0 : _b.valueOf();
console.log(project);
//# sourceMappingURL=index.js.map