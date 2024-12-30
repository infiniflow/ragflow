---
sidebar_position: 3
slug: /text2sql_agent
---

# Create a Text2SQL agent

Build a Text2SQL agent leverging RAGFlow's RAG capabilities. Contributed by @TeslaZY.

## Scenario

The Text2SQL agent is designed to bridge the gap between Natural Language Processing (NLP) and Structured Query Language (SQL). Its key advantages are as follows:

- **Assisting non-technical users with SQL**: Not all users have a background in SQL or understand the structure of the tables involved in queries. With a Text2SQL agent, users can pose questions or request data in natural language without needing an in-depth knowledge of the database structure or SQL syntax.

- **Enhancing SQL development efficiency**: For those familiar with SQL, the Text2SQL agent streamlines the process by enabling users to construct complex queries quickly, without the need to code each part manually.

- **Minimizing errors**: Manually writing SQL queries can be error-prone, particularly for complex queries or for users not well-versed in the database structure. The Text2SQL agent can interpret natural language instructions and generate accurate SQL queries, thereby reducing potential syntax and logic errors.

- **Boosting data analysis capabilities**: In business intelligence and data analysis, swiftly gaining insights from data is critical. The Text2SQL agent facilitates extracting valuable information from databases more directly and conveniently, thus aiding in accelerating decision-making.

- **Automation and integration**: The Text2SQL agent can be integrated into larger systems to support automated workflows, such as automatic report generation and data monitoring. It can also integrate seamlessly with other services and technologies, offering richer application possibilities.

- **Support for multiple languages and varied expressions**: People can express the same idea in numerous ways. An effective Text2SQL system should be capable of understanding diverse expressions and accurately converting them into SQL queries.

In summary, the Text2SQL agent seeks to make database queries more intuitive and user-friendly while ensuring efficiency and accuracy. It caters to a broad spectrum of users, from completely non-technical individuals to seasoned data analysts and developers.

However, traditional Text2SQL solutions often require model fine-tuning, which can substantially escalate deployment and maintenance costs when implemented in enterprise environments alongside RAG or Agent components. RAGFlow’s RAG-based Text2SQL utilizes an existing (connected) large language model (LLM), allowing for seamless integration with other RAG/Agent components without the necessity for additional fine-tuned models.

## Recipe

A list of components required:

- Begin
- Interface
- Retrieval
- Generate
- ExeSQL

## Procedure

### Preparation of Data

#### Database Environment

Mysql-8.0.39

#### Database Table Creation Statements

```sql
SET NAMES utf8mb4;

-- ----------------------------
-- Table structure for Customers
-- ----------------------------
DROP TABLE IF EXISTS `Customers`;
CREATE TABLE `Customers` (
  `CustomerID` int NOT NULL AUTO_INCREMENT,
  `UserName` varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `Email` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `PhoneNumber` varchar(20) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  PRIMARY KEY (`CustomerID`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ----------------------------
-- Table structure for Products
-- ----------------------------
DROP TABLE IF EXISTS `Products`;
CREATE TABLE `Products` (
  `ProductID` int NOT NULL AUTO_INCREMENT,
  `ProductName` varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  `Description` text COLLATE utf8mb4_unicode_ci,
  `Price` decimal(10,2) DEFAULT NULL,
  `StockQuantity` int DEFAULT NULL,
  PRIMARY KEY (`ProductID`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ----------------------------
-- Table structure for Orders
-- ----------------------------
DROP TABLE IF EXISTS `Orders`;
CREATE TABLE `Orders` (
  `OrderID` int NOT NULL AUTO_INCREMENT,
  `CustomerID` int DEFAULT NULL,
  `OrderDate` date DEFAULT NULL,
  `TotalPrice` decimal(10,2) DEFAULT NULL,
  PRIMARY KEY (`OrderID`),
  KEY `CustomerID` (`CustomerID`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ----------------------------
-- Table structure for OrderDetails
-- ----------------------------
DROP TABLE IF EXISTS `OrderDetails`;
CREATE TABLE `OrderDetails` (
  `OrderDetailID` int NOT NULL AUTO_INCREMENT,
  `OrderID` int DEFAULT NULL,
  `ProductID` int DEFAULT NULL,
  `UnitPrice` decimal(10,2) DEFAULT NULL,
  `Quantity` int DEFAULT NULL,
  `TotalPrice` decimal(10,2) DEFAULT NULL,
  PRIMARY KEY (`OrderDetailID`),
  KEY `OrderID` (`OrderID`),
  KEY `ProductID` (`ProductID`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

#### Generate Test Data

```sql
START TRANSACTION;
INSERT INTO Customers (UserName, Email, PhoneNumber) VALUES
('Alice', 'alice@example.com', '123456789'),
('Bob', 'bob@example.com', '987654321'),
('Charlie', 'charlie@example.com', '112233445'),
('Diana', 'diana@example.com', '555666777'),
('Eve', 'eve@example.com', '999888777'),
('Frank', 'frank@example.com', '123123123'),
('Grace', 'grace@example.com', '456456456'),
('Hugo', 'hugo@example.com', '789789789'),
('Ivy', 'ivy@example.com', '321321321'),
('Jack', 'jack@example.com', '654654654');

INSERT INTO Products (ProductName, Description, Price, StockQuantity) VALUES
('Laptop', 'High performance laptop', 1200.00, 50),
('Smartphone', 'Latest model smartphone', 800.00, 100),
('Tablet', 'Portable tablet device', 300.00, 75),
('Headphones', 'Noise-cancelling headphones', 150.00, 200),
('Camera', 'Professional camera', 600.00, 30),
('Monitor', '24-inch Full HD monitor', 200.00, 45),
('Keyboard', 'Mechanical keyboard', 100.00, 150),
('Mouse', 'Ergonomic gaming mouse', 50.00, 250),
('Speaker', 'Wireless Bluetooth speaker', 80.00, 120),
('Router', 'Wi-Fi router with high speed', 120.00, 90);

INSERT INTO Orders (CustomerID, OrderDate, TotalPrice) VALUES
(1, '2024-01-15', 0),
(2, '2024-02-01', 0),
(3, '2024-03-05', 0),
(4, '2024-04-10', 0),
(5, '2024-05-15', 0),
(6, '2024-06-20', 0),
(7, '2024-07-25', 0),
(8, '2024-08-30', 0),
(9, '2024-09-05', 0),
(10, '2024-10-10', 0),
(1, '2024-11-15', 0),
(2, '2024-12-01', 0),
(3, '2024-01-05', 0),
(4, '2024-02-10', 0),
(5, '2024-03-15', 0),
(6, '2024-04-20', 0),
(7, '2024-05-25', 0),
(8, '2024-06-30', 0),
(9, '2024-07-05', 0),
(10, '2024-08-10', 0);

INSERT INTO OrderDetails (OrderID, ProductID, UnitPrice, Quantity, TotalPrice) VALUES
(1, 1, (SELECT Price FROM Products WHERE ProductID = 1), 2, (SELECT Price * 2 FROM Products WHERE ProductID = 1)), 
(1, 2, (SELECT Price FROM Products WHERE ProductID = 2), 1, (SELECT Price FROM Products WHERE ProductID = 2)),
(2, 3, (SELECT Price FROM Products WHERE ProductID = 3), 3, (SELECT Price * 3 FROM Products WHERE ProductID = 3)),
(2, 4, (SELECT Price FROM Products WHERE ProductID = 4), 1, (SELECT Price FROM Products WHERE ProductID = 4)),
(3, 5, (SELECT Price FROM Products WHERE ProductID = 5), 1, (SELECT Price FROM Products WHERE ProductID = 5)),
(3, 6, (SELECT Price FROM Products WHERE ProductID = 6), 2, (SELECT Price * 2 FROM Products WHERE ProductID = 6)),
(4, 7, (SELECT Price FROM Products WHERE ProductID = 7), 5, (SELECT Price * 5 FROM Products WHERE ProductID = 7)),
(5, 8, (SELECT Price FROM Products WHERE ProductID = 8), 3, (SELECT Price * 3 FROM Products WHERE ProductID = 8)),
(5, 9, (SELECT Price FROM Products WHERE ProductID = 9), 2, (SELECT Price * 2 FROM Products WHERE ProductID = 9)),
(6, 10, (SELECT Price FROM Products WHERE ProductID = 10), 4, (SELECT Price * 4 FROM Products WHERE ProductID = 10)),
(7, 2, (SELECT Price FROM Products WHERE ProductID = 2), 4, (SELECT Price * 4 FROM Products WHERE ProductID = 2)),
(7, 8, (SELECT Price FROM Products WHERE ProductID = 8), 3, (SELECT Price * 3 FROM Products WHERE ProductID = 8)),
(8, 1, (SELECT Price FROM Products WHERE ProductID = 1), 1, (SELECT Price FROM Products WHERE ProductID = 1)),
(8, 9, (SELECT Price FROM Products WHERE ProductID = 9), 2, (SELECT Price * 2 FROM Products WHERE ProductID = 9)),
(8, 10, (SELECT Price FROM Products WHERE ProductID = 10), 5, (SELECT Price * 5 FROM Products WHERE ProductID = 10)),
(9, 3, (SELECT Price FROM Products WHERE ProductID = 3), 5, (SELECT Price * 5 FROM Products WHERE ProductID = 3)),
(9, 6, (SELECT Price FROM Products WHERE ProductID = 6), 1, (SELECT Price FROM Products WHERE ProductID = 6)),
(10, 4, (SELECT Price FROM Products WHERE ProductID = 4), 2, (SELECT Price * 2 FROM Products WHERE ProductID = 4)),
(10, 7, (SELECT Price FROM Products WHERE ProductID = 7), 3, (SELECT Price * 3 FROM Products WHERE ProductID = 7)),
(11, 5, (SELECT Price FROM Products WHERE ProductID = 5), 1, (SELECT Price FROM Products WHERE ProductID = 5)),
(11, 10, (SELECT Price FROM Products WHERE ProductID = 10), 4, (SELECT Price * 4 FROM Products WHERE ProductID = 10)),
(12, 1, (SELECT Price FROM Products WHERE ProductID = 1), 3, (SELECT Price * 3 FROM Products WHERE ProductID = 1)),
(12, 8, (SELECT Price FROM Products WHERE ProductID = 8), 2, (SELECT Price * 2 FROM Products WHERE ProductID = 8)),
(13, 2, (SELECT Price FROM Products WHERE ProductID = 2), 1, (SELECT Price FROM Products WHERE ProductID = 2)),
(13, 9, (SELECT Price FROM Products WHERE ProductID = 9), 3, (SELECT Price * 3 FROM Products WHERE ProductID = 9)),
(14, 3, (SELECT Price FROM Products WHERE ProductID = 3), 4, (SELECT Price * 4 FROM Products WHERE ProductID = 3)),
(14, 6, (SELECT Price FROM Products WHERE ProductID = 6), 2, (SELECT Price * 2 FROM Products WHERE ProductID = 6)),
(15, 4, (SELECT Price FROM Products WHERE ProductID = 4), 5, (SELECT Price * 5 FROM Products WHERE ProductID = 4)),
(15, 7, (SELECT Price FROM Products WHERE ProductID = 7), 1, (SELECT Price FROM Products WHERE ProductID = 7)),
(16, 5, (SELECT Price FROM Products WHERE ProductID = 5), 2, (SELECT Price * 2 FROM Products WHERE ProductID = 5)),
(16, 10, (SELECT Price FROM Products WHERE ProductID = 10), 3, (SELECT Price * 3 FROM Products WHERE ProductID = 10)),
(17, 1, (SELECT Price FROM Products WHERE ProductID = 1), 4, (SELECT Price * 4 FROM Products WHERE ProductID = 1)),
(17, 8, (SELECT Price FROM Products WHERE ProductID = 8), 1, (SELECT Price FROM Products WHERE ProductID = 8)),
(18, 2, (SELECT Price FROM Products WHERE ProductID = 2), 5, (SELECT Price * 5 FROM Products WHERE ProductID = 2)),
(18, 9, (SELECT Price FROM Products WHERE ProductID = 9), 2, (SELECT Price * 2 FROM Products WHERE ProductID = 9)),
(19, 3, (SELECT Price FROM Products WHERE ProductID = 3), 3, (SELECT Price * 3 FROM Products WHERE ProductID = 3)),
(19, 6, (SELECT Price FROM Products WHERE ProductID = 6), 4, (SELECT Price * 4 FROM Products WHERE ProductID = 6)),
(20, 4, (SELECT Price FROM Products WHERE ProductID = 4), 1, (SELECT Price FROM Products WHERE ProductID = 4)),
(20, 7, (SELECT Price FROM Products WHERE ProductID = 7), 5, (SELECT Price * 5 FROM Products WHERE ProductID = 7));

-- Update Orders Table's TotalPrice
UPDATE Orders o
JOIN (
    SELECT OrderID, SUM(TotalPrice) as order_total
    FROM OrderDetails
    GROUP BY OrderID
) od ON o.OrderID = od.OrderID
SET o.TotalPrice = od.order_total;

COMMIT;

```
### Configure Knowledge Base

For RAGFlow’s RAG-based Text2SQL, the following knowledge bases are typically required:

- **DDL**: Database table creation statements.
- **DB_Description**: Detailed descriptions of tables and columns.
- **Q->SQL**: Natural language query descriptions along with corresponding SQL query examples (Question-Answer pairs).

However, in specialized query scenarios, user queries might include abbreviations or synonyms for domain-specific terms. If a user references a synonym for a domain-specific term, the system may fail to generate the correct SQL query. Therefore, it is advisable to incorporate a thesaurus for synonyms to assist the Agent in generating more accurate SQL queries.

- **TextSQL_Thesaurus**: A thesaurus covering domain-specific terms and their synonyms.

#### Configure DDL Knowledge Base

1. The content of the DDL text is as follows:
```sql
CREATE TABLE Customers (
  CustomerID int NOT NULL AUTO_INCREMENT,
  UserName varchar(50) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  Email varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  PhoneNumber varchar(20) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  PRIMARY KEY (CustomerID)
) ENGINE=InnoDB AUTO_INCREMENT=11 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE Products (
  ProductID int NOT NULL AUTO_INCREMENT,
  ProductName varchar(100) COLLATE utf8mb4_unicode_ci DEFAULT NULL,
  Description text COLLATE utf8mb4_unicode_ci,
  Price decimal(10,2) DEFAULT NULL,
  StockQuantity int DEFAULT NULL,
  PRIMARY KEY (ProductID)
) ENGINE=InnoDB AUTO_INCREMENT=11 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE Orders (
  OrderID int NOT NULL AUTO_INCREMENT,
  CustomerID int DEFAULT NULL,
  OrderDate date DEFAULT NULL,
  TotalPrice decimal(10,2) DEFAULT NULL,
  PRIMARY KEY (OrderID),
  KEY CustomerID (CustomerID)
) ENGINE=InnoDB AUTO_INCREMENT=21 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE OrderDetails (
  OrderDetailID int NOT NULL AUTO_INCREMENT,
  OrderID int DEFAULT NULL,
  ProductID int DEFAULT NULL,
  UnitPrice decimal(10,2) DEFAULT NULL,
  Quantity int DEFAULT NULL,
  TotalPrice decimal(10,2) DEFAULT NULL,
  PRIMARY KEY (OrderDetailID),
  KEY OrderID (OrderID),
  KEY ProductID (ProductID)
) ENGINE=InnoDB AUTO_INCREMENT=40 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```
2. Set the chunk data for the DLL knowledge base
![DDL knowledge base](https://github.com/user-attachments/assets/2c073e1b-8fdd-443e-98ca-4fd36f9d93e0)

#### Configure DB_Description Knowledge Base

1. the content of the DB_Description text is as follows:
2. 
```markdown
### Customers (Customer Information Table)
The Customers table records detailed information about different customers in the online store. Here is the meaning of each field within this table:
- CustomerID: A unique identifier for a customer, auto-incremented.
- UserName: The name used by the customer for logging into the online store or displayed on the site.
- Email: The email address of the customer, which can be used for account verification, password recovery, and order updates.
- PhoneNumber: The phone number of the customer, useful for contact purposes such as delivery notifications or customer service.

### Products (Product Information Table)
The Products table contains information about the products offered by the online store. Each field within this table represents:
- ProductID: A unique identifier for a product, auto-incremented.
- ProductName: The name of the product, such as laptop, smartphone, nounch, etc.
- Description: Detailed information about the product.
- Price: The selling price of the product, stored as a decimal value to accommodate currency formatting.
- StockQuantity: The quantity of the product available in stock.

### Orders (Order Information Table)
The Orders table tracks orders placed by customers. This table includes fields that denote:
- OrderID: A unique identifier for an order, auto-incremented.
- CustomerID: A foreign key that references the CustomerID in the Customers table, indicating which customer placed the order.
- OrderDate: The date when the order was placed.
- TotalPrice: The total price of all items in the order, calculated at the time of purchase.

### OrderDetails (Order Details Table)
The OrderDetails table provides detailed information about each item in an order. Fields within this table include:
- OrderDetailID: A unique identifier for each line item in an order, auto-incremented.
- OrderID: A foreign key that references the OrderID in the Orders table, linking the detail to a specific order.
- ProductID: A foreign key that references the ProductID in the Products table, specifying which product was ordered.
- UnitPrice: The price per unit of the product at the time of order.
- Quantity: The number of units of the product ordered.
- TotalPrice: The total price for this particular item in the order, calculated as UnitPrice * Quantity.
```

2. set the chunk data for the DB_Description knowledge base
![DB_Description knowledge base](https://github.com/user-attachments/assets/0e3f1cad-dd67-4d7c-ae2d-b31ca3be664d)
#### Configure Q->SQL Knowledge Base
1. Q->SQL Excel Document
[QA.xlsx](https://github.com/user-attachments/files/18258416/QA.xlsx)
2. Upload the Q->SQL Excel document to the Q->SQL knowledge base and set the chunk data as follows via parsing:
![Q->SQL knowledge base](https://github.com/user-attachments/assets/391f4395-1458-4f5b-8b55-517ec3a7b1dc)
#### Configure TextSQL_Thesaurus Knowledge Base
1. the content of the TextSQL_Thesaurus text is as follows:
```txt
###
Standard noun: StockQuantity
Synonyms: stock,stockpile,inventory
###
Standard noun: UserName
Synonyms: user name, user's name
###
Standard noun: Quantity
Synonyms: amount,number
###
Standard noun: Smartphone
Synonyms: phone, mobile phone, smart phone, mobilephone
###
Standard noun: ProductName
Synonyms: product name, product's name
###
Standard noun: tablet
Synonyms: pad,Pad
###
Standard noun: laptop
Synonyms: laptop computer,laptop pc
```
2. set the chunk data for the TextSQL_Thesaurus knowledge base
![TextSQL_Thesaurus knowledge base](https://github.com/user-attachments/assets/76e6f802-1381-4bbc-951f-50992fbeecd8)

### Build the Agent
1. Create an Agent using the Text2SQL Agent template.
2. Enter the configuration page of the Agent to start the setup process.
3. Create a Retrieval node and name it Thesaurus; create an ExeSQL node.
4. Configure the Q->SQL, DDL, DB_Description, and TextSQL_Thesaurus knowledge bases. Please refer to the following:
   ![Configure Retrieval node](https://github.com/user-attachments/assets/25d67b01-954e-4eb4-87f5-c54262cf9a3e)
5. Configure the Generate node, named LLM‘s prompt:
   - Add this content to the prompt provided by the template to provide the thesaurus content to the LLM:
     ```plaintext
     ## You may use the following Thesaurus statements. For example, what I ask is from Synonyms, you must use Standard noun to generate SQL. Use responses to past questions also to guide you: {sql_thesaurus}.
     ```
   - Ensure the mapping between keys and component IDs is configured correctly.
   - The configuration result should look like this:
     ![](https://github.com/user-attachments/assets/04e30e2b-2029-4087-9671-f513dbb1f00d)
6. Configure the ExecSQL node, filling in the configuration information for the MySQL database.
   ![Configure the ExecSQL node](https://github.com/user-attachments/assets/a3173ae7-8a6a-45e1-b9dd-b0311990f48a)
7. Set an opener in the Begin component like:
   ```plaintext
   Hi! I'm your electronic products online store business data analysis assistant. What can I do for you?

### Run and Test the Agent
1. click the Run button to start the agent.
2. input the question:
```
Help me summarize stock quantities for each product
```
3. click the send button to send the question to the agent.
4. The agent will respond with the following:
![](https://github.com/user-attachments/assets/6d31cf09-dd1e-4faa-a7eb-5af6e4ff1d50)


### Debug the Agent

Since version 0.15.0, ragflow has introduced step-by-step execution for Agent components/tools, providing a robust mechanism for debugging and testing. Let's explore how to perform a step run.

1. To enter Test Run mode, you can either click the triangle icon located above the component or access the component's detail page by clicking on the component itself. Once there, select the Test Run button in the upper right corner of the component details.
   ![](https://github.com/user-attachments/assets/9f5d5c3e-396d-418b-9c7a-fcd8ad5af418)
   ![](https://github.com/user-attachments/assets/0816a582-1d88-42a1-bcb1-8f76cb702503)

2. Enter a question that does not exist in the Q->SQL knowledge base but is similar in nature.
   Click the Run button to receive the component's output.

```
Find all customers who has bought a mobile phone
```
![](https://github.com/user-attachments/assets/a6270188-72af-4be7-a192-efddb611f3a4)
3. As the image shows, no matching information was retrieved from the Q->SQL knowledge base, yet a similar question exists within the database. Adjust the Rerank model, "Similarity threshold," or "Keywords similarity weight" accordingly to return relevant content.
![](https://github.com/user-attachments/assets/0592c45b-9276-465d-93d3-2530b2fb81c0)
![](https://github.com/user-attachments/assets/9e72be3a-41af-4ef2-863d-03757ddfdde6)

4. Observe the inputs and outputs of the LLM node and ExeSQL node.
![](https://github.com/user-attachments/assets/55a2b2ec-3518-4fb5-abd5-1634cd485eac)

5. The agent now produces the correct SQL query result.

6. For a query about "mobile phone," the agent successfully generates the appropriate SQL query using "Smartphone." This showcases how the thesaurus guides the LLM in generating accurate SQL queries.

With this, you maybe appreciate the capabilities of Step Run. It undoubtedly assists in constructing more effective agents.

## Troubleshooting
### Total: 0 No record in the database!
1. Confirm if the sql is correct. If so, check the connection information of the database. 
2. If the connection information is correct, maybe there is actually no data matching your query in the database.

## Considerations

In real production scenarios within vertical domains, several considerations are essential for effective Text2SQL implementation:

1. **Handling DDL and DB_Description**: Dealing with Data Definition Language (DDL) statements and database descriptions requires substantial debugging experience. It is crucial to discern which information is vital and which may be redundant, depending on the true business context. This includes determining the relevance of table attributes such as primary keys, foreign keys, indexes, and so forth.

2. **Maintaining Quality QA Data**: Ensuring a high standard for question-and-answer data significantly aids the LLM in generating more accurate SQL queries.

3. **Managing Domain-Specific Synonyms**: Professional domain synonyms can greatly impact the generation of SQL query conditions. Therefore, maintaining an extensive and up-to-date synonym library is critical to mitigate this challenge.

4. **Facilitating User Feedback**: Implementing a feedback mechanism within the Agent allows users to provide correct SQL queries. Administrators can then use this feedback to automatically generate corresponding QA data, reducing the need for manual maintenance.

In summary, achieving high-quality output from Text2SQL remains contingent upon high-quality input. Constructing robust question-and-answer datasets is at the core of optimizing RAGFlow's Text2SQL capabilities.
