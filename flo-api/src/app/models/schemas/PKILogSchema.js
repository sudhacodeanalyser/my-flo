// PKILog Table for DynamoDB

module.exports = {
  TableName : 'PKILog',
  KeySchema: [
    { AttributeName: 'task_id', KeyType: 'HASH' },
    { AttributeName: 'created_at', KeyType: 'RANGE' }
  ],
  AttributeDefinitions: [
    { AttributeName: 'task_id', AttributeType: 'S' },
    { AttributeName: 'created_at', AttributeType: 'S' }
  ],
  ProvisionedThroughput: {
      ReadCapacityUnits: 1,
      WriteCapacityUnits: 1
  }
};
