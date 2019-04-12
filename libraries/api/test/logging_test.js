const request = require('superagent');
const assert = require('assert');
const hawk = require('hawk');
const APIBuilder = require('../');
const helper = require('./helper');
const _ = require('lodash');
const libUrls = require('taskcluster-lib-urls');
const testing = require('taskcluster-lib-testing');
const MonitorManager = require('taskcluster-lib-monitor');

suite(testing.suiteName(), function() {
  // Create test api
  const builder = new APIBuilder({
    title: 'Test Api',
    description: 'Yet another test api',
    serviceName: 'test',
    apiVersion: 'v1',
  });
  const monitorManager = new MonitorManager({
    serviceName: 'foo',
  });
  monitorManager.setup({
    mock: true,
  });
  const monitor = monitorManager.monitor();

  teardown(function() {
    monitorManager.reset();
  });

  // Create a mock authentication server
  setup(async () => {
    await helper.setupServer({builder, monitor});
  });
  teardown(helper.teardownServer);

  builder.declare({
    method: 'get',
    route: '/require-some-scopes',
    name: 'requireSomeScopes',
    title: 'Requre some scopse',
    description: 'Place we can call to test something',
    scopes: {
      AnyOf: [
        {AllOf: ['aa', 'bb']},
        {AllOf: ['aa', 'bb', 'cc']},
        {AllOf: ['bb', 'dd']},
      ],
    },
  }, function(req, res) {
    res.reply({});
  });

  builder.declare({
    method: 'get',
    route: '/require-no-scopes',
    name: 'requireNoScopes',
    title: 'Requre no scopse',
    description: 'Place we can call to test something',
  }, function(req, res) {
    res.reply({});
  });

  builder.declare({
    method: 'get',
    route: '/require-extra-scopes',
    name: 'requireExtraScopes',
    title: 'Requre extra scopse',
    description: 'Place we can call to test something',
    scopes: 'XXXX',
  }, function(req, res) {
    res.reply({});
  });

  test('successful api method is logged', async function() {
    const url = libUrls.api(helper.rootUrl, 'test', 'v1', '/require-some-scopes');
    const {header} = hawk.client.header(url, 'GET', {
      credentials: {id: 'client-with-aa-bb-dd', key: 'ignored', algorithm: 'sha256'},
    });
    await request.get(url).set('Authorization', header);

    assert.equal(monitorManager.messages.length, 1);
    assert(monitorManager.messages[0].Fields.duration > 0); // it exists..
    delete monitorManager.messages[0].Fields.duration;
    assert.deepEqual(monitorManager.messages[0], {
      Type: 'monitor.apiMethod',
      Fields: {
        name: 'requireSomeScopes',
        apiVersion: 'v1',
        clientId: 'client-with-aa-bb-dd',
        // duration handled above
        hasAuthed: true,
        method: 'GET',
        public: false,
        resource: '/api/test/v1/require-some-scopes',
        satisfyingScopes: ['aa', 'bb', 'dd'],
        statusCode: 200,
        v: 1,
      },
      Logger: 'taskcluster.foo.root',
    });
  });

  test('scope-less api method is logged', async function() {
    const url = libUrls.api(helper.rootUrl, 'test', 'v1', '/require-no-scopes');
    const {header} = hawk.client.header(url, 'GET', {
      credentials: {id: 'client-with-aa-bb-dd', key: 'ignored', algorithm: 'sha256'},
    });
    await request.get(url).set('Authorization', header);

    assert.equal(monitorManager.messages.length, 1);
    delete monitorManager.messages[0].Fields.duration;
    assert.deepEqual(monitorManager.messages[0], {
      Type: 'monitor.apiMethod',
      Fields: {
        name: 'requireNoScopes',
        apiVersion: 'v1',
        clientId: '',
        hasAuthed: false,
        method: 'GET',
        public: true,
        resource: '/api/test/v1/require-no-scopes',
        satisfyingScopes: [],
        statusCode: 200,
        v: 1,
      },
      Logger: 'taskcluster.foo.root',
    });
  });

  test('unauthorized api method is logged', async function() {
    const url = libUrls.api(helper.rootUrl, 'test', 'v1', '/require-extra-scopes');
    const {header} = hawk.client.header(url, 'GET', {
      credentials: {id: 'client-with-aa-bb-dd', key: 'ignored', algorithm: 'sha256'},
    });
    try {
      await request.get(url).set('Authorization', header);
    } catch (err) {
      if (err.status !== 403) {
        throw err;
      }
    }

    assert.equal(monitorManager.messages.length, 1);
    delete monitorManager.messages[0].Fields.duration;
    assert.deepEqual(monitorManager.messages[0], {
      Type: 'monitor.apiMethod',
      Fields: {
        name: 'requireExtraScopes',
        apiVersion: 'v1',
        clientId: 'client-with-aa-bb-dd',
        hasAuthed: true,
        method: 'GET',
        public: false,
        resource: '/api/test/v1/require-extra-scopes',
        satisfyingScopes: [],
        statusCode: 403,
        v: 1,
      },
      Logger: 'taskcluster.foo.root',
    });
  });
});
