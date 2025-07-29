-------Begin Transaction------
BEGIN;
SELECT o_id, o_entry_d, o_carrier_id FROM "order" WHERE ((o_w_id = ":-:|'o_w_id','INT8','NOT NULL','PRIMARY KEY','','','','','WHERE','order'|:-:") AND (o_d_id = ":-:|'o_d_id','INT8','NOT NULL','PRIMARY KEY','','','','','WHERE','order'|:-:")) AND (o_c_id = ":-:|'o_c_id','INT8','NULL','','','','','','WHERE','order'|:-:") ORDER BY o_id DESC LIMIT 45;
SELECT c_balance, c_first, c_middle, c_last FROM customer WHERE ((c_w_id = ":-:|'c_w_id','INT8','NOT NULL','PRIMARY KEY','','','','','WHERE','customer'|:-:") AND (c_d_id = ":-:|'c_d_id','INT8','NOT NULL','PRIMARY KEY','','','','','WHERE','customer'|:-:")) AND (c_id = ":-:|'c_id','INT8','NOT NULL','PRIMARY KEY','','','','','WHERE','customer'|:-:");
SELECT ol_i_id, ol_supply_w_id, ol_quantity, ol_amount, ol_delivery_d FROM order_line WHERE ((ol_w_id = ":-:|'ol_w_id','INT8','NOT NULL','PRIMARY KEY','','','','','WHERE','order_line'|:-:") AND (ol_d_id = ":-:|'ol_d_id','INT8','NOT NULL','PRIMARY KEY','','','','','WHERE','order_line'|:-:")) AND (ol_o_id = ":-:|'ol_o_id','INT8','NOT NULL','PRIMARY KEY','','','','','WHERE','order_line'|:-:");
COMMIT;
-------End Transaction-------
-------Begin Transaction------
BEGIN;
SELECT o_id, o_entry_d, o_carrier_id FROM "order" WHERE ((o_w_id = ":-:|'o_w_id','INT8','NOT NULL','PRIMARY KEY','','','','','WHERE','order'|:-:") AND (o_d_id = ":-:|'o_d_id','INT8','NOT NULL','PRIMARY KEY','','','','','WHERE','order'|:-:")) AND (o_c_id = ":-:|'o_c_id','INT8','NULL','','','','','','WHERE','order'|:-:") ORDER BY o_id DESC LIMIT 26;
SELECT c_id, c_balance, c_first, c_middle FROM customer WHERE ((c_w_id = ":-:|'c_w_id','INT8','NOT NULL','PRIMARY KEY','','','','','WHERE','customer'|:-:") AND (c_d_id = ":-:|'c_d_id','INT8','NOT NULL','PRIMARY KEY','','','','','WHERE','customer'|:-:")) AND (c_last = ":-:|'c_last','VARCHAR(16)','NOT NULL','','','','','','WHERE','customer'|:-:") ORDER BY c_first ASC;
SELECT c_id, c_balance, c_first, c_middle FROM customer WHERE ((c_w_id = ":-:|'c_w_id','INT8','NOT NULL','PRIMARY KEY','','','','','WHERE','customer'|:-:") AND (c_d_id = ":-:|'c_d_id','INT8','NOT NULL','PRIMARY KEY','','','','','WHERE','customer'|:-:")) AND (c_last = ":-:|'c_last','VARCHAR(16)','NOT NULL','','','','','','WHERE','customer'|:-:") ORDER BY c_first ASC;
SELECT ol_i_id, ol_supply_w_id, ol_quantity, ol_amount, ol_delivery_d FROM order_line WHERE ((ol_w_id = ":-:|'ol_w_id','INT8','NOT NULL','PRIMARY KEY','','','','','WHERE','order_line'|:-:") AND (ol_d_id = ":-:|'ol_d_id','INT8','NOT NULL','PRIMARY KEY','','','','','WHERE','order_line'|:-:")) AND (ol_o_id = ":-:|'ol_o_id','INT8','NOT NULL','PRIMARY KEY','','','','','WHERE','order_line'|:-:");
COMMIT;
-------End Transaction-------
-------Begin Transaction------
BEGIN;
SELECT d_next_o_id FROM district WHERE (d_w_id = ":-:|'d_w_id','INT8','NOT NULL','PRIMARY KEY','','','','','WHERE','district'|:-:") AND (d_id = ":-:|'d_id','INT8','NOT NULL','PRIMARY KEY','','','','','WHERE','district'|:-:");
SELECT count(DISTINCT s_i_id) FROM order_line JOIN stock ON (s_w_id = ":-:|'s_w_id','INT8','NOT NULL','PRIMARY KEY','','','','','WHERE','stock'|:-:") AND (s_i_id = ol_i_id) WHERE (((ol_w_id = ":-:|'ol_w_id','INT8','NOT NULL','PRIMARY KEY','','','','','WHERE','order_line'|:-:") AND (ol_d_id = ":-:|'ol_d_id','INT8','NOT NULL','PRIMARY KEY','','','','','WHERE','order_line'|:-:")) AND (ol_o_id BETWEEN (":-:|'ol_o_id','INT8','NOT NULL','PRIMARY KEY','','','','','WHERE','order_line'|:-:") AND (":-:|'ol_o_id','INT8','NOT NULL','PRIMARY KEY','','','','','WHERE','order_line'|:-:"))) AND (s_quantity < ":-:|'s_quantity','INT8','NULL','','','','','','WHERE','stock'|:-:");
COMMIT;
-------End Transaction-------
